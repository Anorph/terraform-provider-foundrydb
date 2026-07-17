package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure edgeNodeResource satisfies resource.Resource.
var _ resource.Resource = &edgeNodeResource{}

// edgeNodeCreateTimeout bounds how long Create waits for a freshly provisioned
// PoP to reach a running state before returning an error.
const edgeNodeCreateTimeout = 20 * time.Minute

// edgeNodePollInterval is the delay between status polls during Create.
const edgeNodePollInterval = 10 * time.Second

// edgeNodeResource implements the foundrydb_edge_node resource.
type edgeNodeResource struct {
	edge *edgeClient
}

// edgeNodeResourceModel holds the Terraform state for a foundrydb_edge_node.
type edgeNodeResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Zone            types.String `tfsdk:"zone"`
	Plan            types.String `tfsdk:"plan"`
	TargetNodeCount types.Int64  `tfsdk:"target_node_count"`
	Name            types.String `tfsdk:"name"`
	Status          types.String `tfsdk:"status"`
	NodeCount       types.Int64  `tfsdk:"node_count"`
}

// NewEdgeNodeResource returns a new edgeNodeResource factory.
func NewEdgeNodeResource() resource.Resource {
	return &edgeNodeResource{}
}

func (r *edgeNodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_edge_node"
}

func (r *edgeNodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Provisions and scales an edge point-of-presence (PoP) in the FoundryDB edge tier. A PoP is one or more edge VMs in a single zone; a `target_node_count` of 2 or more runs a primary that holds the serving floating IP plus one or more hot standbys. The edge state machine converges the live `node_count` toward `target_node_count` asynchronously. Changing `target_node_count` scales the PoP in place; changing `zone` or `plan` destroys and recreates it. This resource models the declarative surface of the edge fleet only; the imperative roll operation and the read-only fleet overview and recovery views are handled through the SDK and CLI.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier (UUID) of the edge PoP.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"zone": schema.StringAttribute{
				MarkdownDescription: "Cloud zone the PoP is provisioned in (e.g. `se-sto1`). Changing this value destroys and recreates the resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plan": schema.StringAttribute{
				MarkdownDescription: "Compute plan for the PoP's edge VMs. Optional; when omitted the platform selects the default edge plan. Changing this value destroys and recreates the resource.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_node_count": schema.Int64Attribute{
				MarkdownDescription: "Desired number of edge VMs in the PoP. `1` is a single-node PoP; `2` or more provisions a primary plus hot standbys for high availability. Defaults to `2`. Changing this value scales the PoP in place.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Platform-assigned name of the PoP.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current lifecycle status of the PoP as reported by the edge state machine (e.g. `provisioning`, `running`, `scaling`, `failed`).",
				Computed:            true,
			},
			"node_count": schema.Int64Attribute{
				MarkdownDescription: "Number of edge VMs currently live in the PoP. Converges toward `target_node_count` as the edge state machine reconciles.",
				Computed:            true,
			},
		},
	}
}

func (r *edgeNodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected resource configure type",
			fmt.Sprintf("Expected *providerData, got %T", req.ProviderData),
		)
		return
	}
	r.edge = pd.edgeClient
}

// edgeNodeDefaultTargetNodeCount is the desired VM count applied when the
// argument is omitted: a primary plus one hot standby.
const edgeNodeDefaultTargetNodeCount = 2

func (r *edgeNodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan edgeNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	target := int64(edgeNodeDefaultTargetNodeCount)
	if !plan.TargetNodeCount.IsNull() && !plan.TargetNodeCount.IsUnknown() {
		target = plan.TargetNodeCount.ValueInt64()
	}

	planName := ""
	if !plan.Plan.IsNull() && !plan.Plan.IsUnknown() {
		planName = plan.Plan.ValueString()
	}

	node, err := r.edge.CreateEdgeNode(ctx, plan.Zone.ValueString(), planName, int(target))
	if err != nil {
		resp.Diagnostics.AddError("Error creating edge PoP", err.Error())
		return
	}

	// The edge state machine converges asynchronously; wait for the PoP to reach
	// a running state (bounded) before returning so that subsequent plans see a
	// stable node_count.
	node, err = r.pollEdgeNodeUntilRunning(ctx, node.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for edge PoP to become running", err.Error())
		return
	}

	edgeNodeToState(node, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *edgeNodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state edgeNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	node, err := r.edge.GetEdgeNode(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading edge PoP", err.Error())
		return
	}
	if node == nil {
		// PoP was deprovisioned outside of Terraform.
		resp.State.RemoveResource(ctx)
		return
	}

	edgeNodeToState(node, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *edgeNodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan edgeNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state edgeNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	target := state.TargetNodeCount.ValueInt64()
	if !plan.TargetNodeCount.IsNull() && !plan.TargetNodeCount.IsUnknown() {
		target = plan.TargetNodeCount.ValueInt64()
	}

	node, err := r.edge.ScaleEdgeNode(ctx, state.ID.ValueString(), int(target))
	if err != nil {
		resp.Diagnostics.AddError("Error scaling edge PoP", err.Error())
		return
	}

	edgeNodeToState(node, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *edgeNodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state edgeNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.edge.DeleteEdgeNode(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting edge PoP", err.Error())
	}
}

// edgeNodeRunningStates and edgeNodeFailedStates classify the terminal outcomes
// the create poll waits for. Matching is case-insensitive.
var edgeNodeRunningStates = map[string]bool{
	"running": true,
	"active":  true,
	"ready":   true,
	"serving": true,
}

var edgeNodeFailedStates = map[string]bool{
	"failed": true,
	"error":  true,
}

// pollEdgeNodeUntilRunning polls the fleet list until the PoP reaches a running
// state or fails. It times out after edgeNodeCreateTimeout.
func (r *edgeNodeResource) pollEdgeNodeUntilRunning(ctx context.Context, id string) (*EdgeNode, error) {
	deadline := time.Now().Add(edgeNodeCreateTimeout)
	for {
		node, err := r.edge.GetEdgeNode(ctx, id)
		if err != nil {
			return nil, err
		}
		if node == nil {
			return nil, fmt.Errorf("edge PoP %q disappeared while waiting for it to become running", id)
		}

		status := strings.ToLower(strings.TrimSpace(node.Status))
		if edgeNodeRunningStates[status] {
			return node, nil
		}
		if edgeNodeFailedStates[status] {
			return nil, fmt.Errorf("edge PoP %q provisioning failed (status: %s)", id, node.Status)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for edge PoP %q to become running (current status: %s)", id, node.Status)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(edgeNodePollInterval):
		}
	}
}

// edgeNodeToState maps an API EdgeNode into the Terraform state model.
func edgeNodeToState(n *EdgeNode, model *edgeNodeResourceModel) {
	model.ID = types.StringValue(n.ID)
	model.Zone = types.StringValue(n.Zone)
	model.Plan = types.StringValue(n.Plan)
	model.TargetNodeCount = types.Int64Value(int64(n.TargetNodeCount))
	model.Name = types.StringValue(n.Name)
	model.Status = types.StringValue(n.Status)
	model.NodeCount = types.Int64Value(int64(n.NodeCount))
}
