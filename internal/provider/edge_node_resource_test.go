package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anorph/terraform-provider-foundrydb/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// edgeNodeResponse builds a minimal API JSON body for an EdgeNode.
func edgeNodeResponse(id, name, zone, plan, status string, nodeCount, targetNodeCount int) map[string]interface{} {
	return map[string]interface{}{
		"id":                id,
		"name":              name,
		"zone":              zone,
		"plan":              plan,
		"status":            status,
		"node_count":        nodeCount,
		"target_node_count": targetNodeCount,
		"created_at":        "2026-01-01T00:00:00Z",
		"updated_at":        "2026-01-01T00:00:00Z",
	}
}

// configuredEdgeNodeResource returns an edgeNodeResource with a providerData
// configured against the provided httptest server URL.
func configuredEdgeNodeResource(t *testing.T, apiURL string) resource.Resource {
	t.Helper()
	r := provider.NewEdgeNodeResource()
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("NewEdgeNodeResource() does not implement ResourceWithConfigure")
	}
	pd := provider.NewProviderDataForTest(apiURL, "admin", "admin")
	req := resource.ConfigureRequest{ProviderData: pd}
	resp := &resource.ConfigureResponse{}
	configurable.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", resp.Diagnostics)
	}
	return r
}

// getEdgeNodeSchema returns the schema for the edge node resource.
func getEdgeNodeSchema(t *testing.T, r resource.Resource) resourceschema.Schema {
	t.Helper()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() failed: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// edgeNodeStateModel mirrors edgeNodeResourceModel for state decoding in tests.
type edgeNodeStateModel struct {
	ID              types.String `tfsdk:"id"`
	Zone            types.String `tfsdk:"zone"`
	Plan            types.String `tfsdk:"plan"`
	TargetNodeCount types.Int64  `tfsdk:"target_node_count"`
	Name            types.String `tfsdk:"name"`
	Status          types.String `tfsdk:"status"`
	NodeCount       types.Int64  `tfsdk:"node_count"`
}

// TestUnitEdgeNodeResource_Metadata verifies the resource type name.
func TestUnitEdgeNodeResource_Metadata(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	req := resource.MetadataRequest{ProviderTypeName: "foundrydb"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "foundrydb_edge_node" {
		t.Errorf("TypeName = %q; want %q", resp.TypeName, "foundrydb_edge_node")
	}
}

// TestUnitEdgeNodeResource_Schema_requiredAttributes verifies required attributes.
func TestUnitEdgeNodeResource_Schema_requiredAttributes(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() returned errors: %v", resp.Diagnostics)
	}

	attr, ok := resp.Schema.Attributes["zone"]
	if !ok {
		t.Fatal("schema missing required attribute \"zone\"")
	}
	if !attr.IsRequired() {
		t.Error("attribute \"zone\" should be Required")
	}
}

// TestUnitEdgeNodeResource_Schema_computedAttributes verifies computed attributes.
func TestUnitEdgeNodeResource_Schema_computedAttributes(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for _, key := range []string{"id", "name", "status", "node_count"} {
		attr, ok := resp.Schema.Attributes[key]
		if !ok {
			t.Errorf("schema missing computed attribute %q", key)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("attribute %q should be Computed", key)
		}
	}
}

// TestUnitEdgeNodeResource_Schema_optionalAttributes verifies optional-updatable attributes.
func TestUnitEdgeNodeResource_Schema_optionalAttributes(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for _, key := range []string{"plan", "target_node_count"} {
		attr, ok := resp.Schema.Attributes[key]
		if !ok {
			t.Errorf("schema missing optional attribute %q", key)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("attribute %q should be Optional", key)
		}
	}
}

// TestUnitEdgeNodeResource_Schema_allExpectedFields verifies all expected attributes exist.
func TestUnitEdgeNodeResource_Schema_allExpectedFields(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for _, field := range []string{"id", "zone", "plan", "target_node_count", "name", "status", "node_count"} {
		if _, ok := resp.Schema.Attributes[field]; !ok {
			t.Errorf("expected attribute %q not found in edge node resource schema", field)
		}
	}
}

// TestUnitEdgeNodeResource_Schema_markdownDescription verifies schema description.
func TestUnitEdgeNodeResource_Schema_markdownDescription(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.MarkdownDescription == "" {
		t.Error("edge node resource schema MarkdownDescription should not be empty")
	}
}

// TestUnitEdgeNodeResource_Configure_nilProviderData verifies Configure handles nil data.
func TestUnitEdgeNodeResource_Configure_nilProviderData(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("NewEdgeNodeResource() does not implement ResourceWithConfigure")
	}

	req := resource.ConfigureRequest{ProviderData: nil}
	resp := &resource.ConfigureResponse{}
	configurable.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("Configure with nil provider data should not produce errors; got: %v", resp.Diagnostics)
	}
}

// TestUnitEdgeNodeResource_Configure_wrongType verifies Configure errors on wrong type.
func TestUnitEdgeNodeResource_Configure_wrongType(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()

	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("NewEdgeNodeResource() does not implement ResourceWithConfigure")
	}

	req := resource.ConfigureRequest{ProviderData: "not-a-providerData"}
	resp := &resource.ConfigureResponse{}
	configurable.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("Configure with wrong provider data type should produce an error")
	}
}

// TestUnitEdgeNodeCRUD_Create_success verifies Create POSTs to /admin/edge/nodes,
// polls the fleet list, and sets state once the PoP reports running.
func TestUnitEdgeNodeCRUD_Create_success(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-001"
	const zone = "se-sto1"
	var createCalled atomic.Bool
	var scaleRequested atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/edge/nodes":
			createCalled.Store(true)
			var body struct {
				Zone      string `json:"zone"`
				Plan      string `json:"plan"`
				NodeCount int    `json:"node_count"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scaleRequested.Store(int64(body.NodeCount))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write(jsonBody(edgeNodeResponse(nodeID, "edge-se-sto1-001", zone, "edge-tier-1", "provisioning", 0, body.NodeCount)))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/admin/edge/nodes":
			// Poll list: report the PoP as already running so Create returns fast.
			body := map[string]interface{}{
				"nodes": []map[string]interface{}{
					edgeNodeResponse(nodeID, "edge-se-sto1-001", zone, "edge-tier-1", "running", 3, 3),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(body)
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)

	plan := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"zone":              tftypes.NewValue(tftypes.String, zone),
		"target_node_count": tftypes.NewValue(tftypes.Number, 3),
	})
	initialState := buildNullState(t, schema)

	resp := &resource.CreateResponse{State: tfsdk.State(initialState)}
	res.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan(plan)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned errors: %v", resp.Diagnostics)
	}
	if !createCalled.Load() {
		t.Error("POST request was not sent to /admin/edge/nodes")
	}
	if got := scaleRequested.Load(); got != 3 {
		t.Errorf("create node_count = %d; want 3", got)
	}

	var got edgeNodeStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("State.Get failed: %v", diags)
	}

	assertEq(t, "id", nodeID, got.ID.ValueString())
	assertEq(t, "zone", zone, got.Zone.ValueString())
	assertEq(t, "status", "running", got.Status.ValueString())
	if got.NodeCount.ValueInt64() != 3 {
		t.Errorf("node_count = %d; want 3", got.NodeCount.ValueInt64())
	}
	if got.TargetNodeCount.ValueInt64() != 3 {
		t.Errorf("target_node_count = %d; want 3", got.TargetNodeCount.ValueInt64())
	}
}

// TestUnitEdgeNodeCRUD_Create_defaultTarget verifies Create sends the default
// target_node_count when the argument is omitted.
func TestUnitEdgeNodeCRUD_Create_defaultTarget(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-def"
	var nodeCountSent atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/edge/nodes":
			var body struct {
				NodeCount int `json:"node_count"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			nodeCountSent.Store(int64(body.NodeCount))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write(jsonBody(edgeNodeResponse(nodeID, "edge-x", "se-sto1", "edge-tier-1", "provisioning", 0, body.NodeCount)))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/admin/edge/nodes":
			body := map[string]interface{}{
				"nodes": []map[string]interface{}{
					edgeNodeResponse(nodeID, "edge-x", "se-sto1", "edge-tier-1", "running", 2, 2),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(body)
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)

	plan := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})
	initialState := buildNullState(t, schema)

	resp := &resource.CreateResponse{State: tfsdk.State(initialState)}
	res.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan(plan)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned errors: %v", resp.Diagnostics)
	}
	if got := nodeCountSent.Load(); got != 2 {
		t.Errorf("default node_count = %d; want 2", got)
	}
}

// TestUnitEdgeNodeCRUD_Create_apiError verifies Create surfaces API errors.
func TestUnitEdgeNodeCRUD_Create_apiError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":"zone not available"}`))
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)

	plan := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"zone": tftypes.NewValue(tftypes.String, "bad-zone"),
	})
	initialState := buildNullState(t, schema)

	resp := &resource.CreateResponse{State: tfsdk.State(initialState)}
	res.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan(plan)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("Create should return a diagnostic error when the API returns 422")
	}
}

// TestUnitEdgeNodeCRUD_Create_failedStatus verifies Create errors when the PoP
// reaches a failed terminal state during the poll.
func TestUnitEdgeNodeCRUD_Create_failedStatus(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-fail"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/edge/nodes":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write(jsonBody(edgeNodeResponse(nodeID, "edge-f", "se-sto1", "edge-tier-1", "provisioning", 0, 2)))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/admin/edge/nodes":
			body := map[string]interface{}{
				"nodes": []map[string]interface{}{
					edgeNodeResponse(nodeID, "edge-f", "se-sto1", "edge-tier-1", "failed", 0, 2),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(body)
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)

	plan := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})
	initialState := buildNullState(t, schema)

	resp := &resource.CreateResponse{State: tfsdk.State(initialState)}
	res.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan(plan)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("Create should return a diagnostic error when the PoP reaches a failed state")
	}
}

// TestUnitEdgeNodeCRUD_Read_success verifies Read finds the PoP in the fleet list.
func TestUnitEdgeNodeCRUD_Read_success(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-002"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/admin/edge/nodes" {
			body := map[string]interface{}{
				"nodes": []map[string]interface{}{
					edgeNodeResponse("other-pop", "edge-other", "fi-hel1", "edge-tier-1", "running", 1, 1),
					edgeNodeResponse(nodeID, "edge-se-sto1-002", "se-sto1", "edge-tier-2", "running", 2, 2),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(body)
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)
	state := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, nodeID),
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})

	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read returned errors: %v", resp.Diagnostics)
	}

	var got edgeNodeStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("State.Get failed: %v", diags)
	}

	assertEq(t, "id", nodeID, got.ID.ValueString())
	assertEq(t, "zone", "se-sto1", got.Zone.ValueString())
	assertEq(t, "plan", "edge-tier-2", got.Plan.ValueString())
	assertEq(t, "status", "running", got.Status.ValueString())
	if got.NodeCount.ValueInt64() != 2 {
		t.Errorf("node_count = %d; want 2", got.NodeCount.ValueInt64())
	}
}

// TestUnitEdgeNodeCRUD_Read_notFound verifies Read removes state when the PoP is gone.
func TestUnitEdgeNodeCRUD_Read_notFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]interface{}{"nodes": []interface{}{}}
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(body)
		w.Write(b)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)
	state := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "edge-gone-003"),
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})

	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not return errors when PoP is not found; got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be null when PoP is not found (resource removed)")
	}
}

// TestUnitEdgeNodeCRUD_Read_apiError verifies Read propagates API errors.
func TestUnitEdgeNodeCRUD_Read_apiError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)
	state := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "err-pop-001"),
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})

	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("Read should return a diagnostic error when the API returns 500")
	}
}

// TestUnitEdgeNodeCRUD_Update_success verifies Update PATCHes the scale call and
// updates state with the new target.
func TestUnitEdgeNodeCRUD_Update_success(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-scale"
	var patchReceived atomic.Bool
	var targetSent atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/admin/edge/nodes/"+nodeID {
			patchReceived.Store(true)
			var body struct {
				TargetNodeCount int `json:"target_node_count"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			targetSent.Store(int64(body.TargetNodeCount))
			w.Header().Set("Content-Type", "application/json")
			w.Write(jsonBody(edgeNodeResponse(nodeID, "edge-s", "se-sto1", "edge-tier-1", "scaling", 2, body.TargetNodeCount)))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)

	plan := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, nodeID),
		"zone":              tftypes.NewValue(tftypes.String, "se-sto1"),
		"target_node_count": tftypes.NewValue(tftypes.Number, 4),
	})
	curState := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, nodeID),
		"zone":              tftypes.NewValue(tftypes.String, "se-sto1"),
		"target_node_count": tftypes.NewValue(tftypes.Number, 2),
	})

	initialResp := buildNullState(t, schema)
	resp := &resource.UpdateResponse{State: tfsdk.State(initialResp)}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan(plan),
		State: curState,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update returned errors: %v", resp.Diagnostics)
	}
	if !patchReceived.Load() {
		t.Error("PATCH request was not sent to the API")
	}
	if got := targetSent.Load(); got != 4 {
		t.Errorf("scale target_node_count = %d; want 4", got)
	}

	var got edgeNodeStateModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("State.Get after Update failed: %v", diags)
	}
	if got.TargetNodeCount.ValueInt64() != 4 {
		t.Errorf("target_node_count after update = %d; want 4", got.TargetNodeCount.ValueInt64())
	}
}

// TestUnitEdgeNodeCRUD_Update_apiError verifies Update surfaces PATCH errors.
func TestUnitEdgeNodeCRUD_Update_apiError(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-scale-err"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":"target_node_count out of range"}`))
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)

	plan := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, nodeID),
		"zone":              tftypes.NewValue(tftypes.String, "se-sto1"),
		"target_node_count": tftypes.NewValue(tftypes.Number, 99),
	})
	curState := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, nodeID),
		"zone":              tftypes.NewValue(tftypes.String, "se-sto1"),
		"target_node_count": tftypes.NewValue(tftypes.Number, 2),
	})

	initialResp := buildNullState(t, schema)
	resp := &resource.UpdateResponse{State: tfsdk.State(initialResp)}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan(plan),
		State: curState,
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("Update should return a diagnostic error when the API returns 422")
	}
}

// TestUnitEdgeNodeCRUD_Delete_success verifies Delete calls DELETE /admin/edge/nodes/:id.
func TestUnitEdgeNodeCRUD_Delete_success(t *testing.T) {
	t.Parallel()

	const nodeID = "edge-pop-del"
	var deleted atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/admin/edge/nodes/"+nodeID {
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)
	state := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, nodeID),
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})

	resp := &resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete returned errors: %v", resp.Diagnostics)
	}
	if !deleted.Load() {
		t.Error("DELETE request was not sent to the API")
	}
}

// TestUnitEdgeNodeCRUD_Delete_notFound verifies Delete treats 404 as success (idempotent).
func TestUnitEdgeNodeCRUD_Delete_notFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	res := configuredEdgeNodeResource(t, srv.URL)
	schema := getEdgeNodeSchema(t, res)
	state := buildStateWithAttrs(t, schema, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "already-gone-pop"),
		"zone": tftypes.NewValue(tftypes.String, "se-sto1"),
	})

	resp := &resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should treat 404 as success; got errors: %v", resp.Diagnostics)
	}
}

// TestUnitEdgeNodeResource_NewEdgeNodeResourceNotNil verifies the constructor returns non-nil.
func TestUnitEdgeNodeResource_NewEdgeNodeResourceNotNil(t *testing.T) {
	t.Parallel()
	r := provider.NewEdgeNodeResource()
	if r == nil {
		t.Fatal("NewEdgeNodeResource() returned nil")
	}
}
