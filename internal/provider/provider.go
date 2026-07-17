package provider

import (
	"context"

	"github.com/anorph/foundrydb-sdk-go/foundrydb"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure foundrydbProvider satisfies the provider.Provider interface.
var _ provider.Provider = &foundrydbProvider{}

// foundrydbProvider is the provider implementation.
type foundrydbProvider struct {
	version string
}

// foundrydbProviderModel holds the provider configuration values.
type foundrydbProviderModel struct {
	APIURL   types.String `tfsdk:"api_url"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// providerData bundles the SDK client together with the local edge client and
// is passed as ResourceData and DataSourceData to every resource and data
// source. Resources that pre-date the edge tier also accept a bare
// *foundrydb.Client for backward compatibility with existing unit tests.
type providerData struct {
	client     *foundrydb.Client
	edgeClient *edgeClient
}

// NewProviderDataForTest constructs a providerData from explicit credentials.
// Intended for use in package-external unit tests only.
func NewProviderDataForTest(apiURL, username, password string) *providerData {
	c := foundrydb.New(foundrydb.Config{
		APIURL:   apiURL,
		Username: username,
		Password: password,
	})
	return &providerData{
		client:     c,
		edgeClient: newEdgeClient(apiURL, username, password),
	}
}

// New returns a provider factory function.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &foundrydbProvider{version: version}
	}
}

func (p *foundrydbProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "foundrydb"
	resp.Version = p.version
}

func (p *foundrydbProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for the FoundryDB managed database platform. Manage PostgreSQL, MySQL, MongoDB, Valkey, Kafka, OpenSearch, and MSSQL clusters on UpCloud infrastructure.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				MarkdownDescription: "Base URL of the FoundryDB controller API. Defaults to `https://api.foundrydb.com`.",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for Basic Auth authentication against the FoundryDB API.",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for Basic Auth authentication against the FoundryDB API.",
				Required:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *foundrydbProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config foundrydbProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiURL := "https://api.foundrydb.com"
	if !config.APIURL.IsNull() && !config.APIURL.IsUnknown() && config.APIURL.ValueString() != "" {
		apiURL = config.APIURL.ValueString()
	}

	username := config.Username.ValueString()
	password := config.Password.ValueString()

	pd := &providerData{
		client: foundrydb.New(foundrydb.Config{
			APIURL:   apiURL,
			Username: username,
			Password: password,
		}),
		edgeClient: newEdgeClient(apiURL, username, password),
	}
	resp.DataSourceData = pd
	resp.ResourceData = pd
}

func (p *foundrydbProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewServiceResource,
		NewAppServiceResource,
		NewAppServiceAuthResource,
		NewAppJobResource,
		NewQueueResource,
		NewEdgeDomainResource,
		NewEdgeNodeResource,
		NewAppEdgeSettingsResource,
		NewWebhookResource,
		NewDataPipelineResource,
		NewStackResource,
		NewStackTemplateResource,
		NewAttachmentResource,
	}
}

func (p *foundrydbProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDatabaseUserDataSource,
		NewOrganizationsDataSource,
		NewStackTemplatesDataSource,
	}
}
