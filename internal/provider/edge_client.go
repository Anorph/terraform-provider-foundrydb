package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// edgeClient is a minimal HTTP client for the Edge Gateway API surface that is
// not yet present in the vendored foundrydb-sdk-go. It uses the same Basic Auth
// + Content-Type conventions as the SDK client.
type edgeClient struct {
	apiURL     string
	username   string
	password   string
	httpClient *http.Client
}

// newEdgeClient constructs an edgeClient from the provider config fields.
func newEdgeClient(apiURL, username, password string) *edgeClient {
	u := strings.TrimRight(apiURL, "/")
	if u == "" {
		u = "https://api.foundrydb.com"
	}
	return &edgeClient{
		apiURL:   u,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do executes an authenticated HTTP request and returns the raw response.
func (c *edgeClient) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("edge: marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("edge: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("edge: request %s %s: %w", method, path, err)
	}
	return resp, nil
}

// checkResp reads the response body. On a non-2xx status it returns an error
// with the status code and message; on success it returns the raw body bytes.
func (c *edgeClient) checkResp(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edge: read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := extractEdgeErrorMessage(data)
		return nil, fmt.Errorf("edge: API error %d: %s", resp.StatusCode, msg)
	}
	return data, nil
}

func extractEdgeErrorMessage(body []byte) string {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if payload.Error != "" {
			return payload.Error
		}
		if payload.Message != "" {
			return payload.Message
		}
	}
	raw := strings.TrimSpace(string(body))
	if len(raw) > 200 {
		raw = raw[:200] + "..."
	}
	return raw
}

// EdgeDomain is a customer custom domain attached to an app service.
type EdgeDomain struct {
	ID          string `json:"id"`
	ServiceID   string `json:"service_id"`
	Domain      string `json:"domain"`
	Status      string `json:"status"`
	CNAMETarget string `json:"cname_target,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type createEdgeDomainRequest struct {
	Domain string `json:"domain"`
}

type listEdgeDomainsResponse struct {
	Domains []EdgeDomain `json:"domains"`
}

// CreateDomain attaches a custom domain to an app service.
func (c *edgeClient) CreateDomain(ctx context.Context, appServiceID string, domain string) (*EdgeDomain, error) {
	resp, err := c.do(ctx, http.MethodPost, "/app-services/"+appServiceID+"/domains", createEdgeDomainRequest{Domain: domain})
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var d EdgeDomain
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("edge: decode CreateDomain response: %w", err)
	}
	return &d, nil
}

// ListDomains returns all custom domains attached to an app service.
func (c *edgeClient) ListDomains(ctx context.Context, appServiceID string) ([]EdgeDomain, error) {
	resp, err := c.do(ctx, http.MethodGet, "/app-services/"+appServiceID+"/domains", nil)
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var result listEdgeDomainsResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("edge: decode ListDomains response: %w", err)
	}
	return result.Domains, nil
}

// GetDomain finds a domain by ID within the app service's domain list. Returns
// nil when no domain with that ID exists (treats it as deleted).
func (c *edgeClient) GetDomain(ctx context.Context, appServiceID, domainID string) (*EdgeDomain, error) {
	domains, err := c.ListDomains(ctx, appServiceID)
	if err != nil {
		return nil, err
	}
	for i := range domains {
		if domains[i].ID == domainID {
			return &domains[i], nil
		}
	}
	return nil, nil
}

// DeleteDomain removes a custom domain from an app service. A 404 is treated
// as success (idempotent).
func (c *edgeClient) DeleteDomain(ctx context.Context, appServiceID, domainID string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/app-services/"+appServiceID+"/domains/"+domainID, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil
	}
	_, err = c.checkResp(resp)
	return err
}

// EdgeCacheKey narrows the cache key for a cache rule to a chosen set of query
// parameters, request headers, and cookies. An empty EdgeCacheKey caches under
// the full request URL.
type EdgeCacheKey struct {
	VaryQueryParams []string `json:"vary_query_params,omitempty"`
	VaryHeaders     []string `json:"vary_headers,omitempty"`
	VaryCookies     []string `json:"vary_cookies,omitempty"`
}

// EdgeCacheRule caches responses under one path prefix for a fixed TTL, with
// optional stale-serving windows, request collapsing, and a narrowed cache key.
type EdgeCacheRule struct {
	PathPrefix                  string        `json:"path_prefix"`
	TTLSeconds                  int           `json:"ttl_seconds"`
	StaleWhileRevalidateSeconds int           `json:"stale_while_revalidate_seconds,omitempty"`
	StaleIfErrorSeconds         int           `json:"stale_if_error_seconds,omitempty"`
	CacheKey                    *EdgeCacheKey `json:"cache_key,omitempty"`
	RequestCollapsing           bool          `json:"request_collapsing,omitempty"`
}

// EdgeRateLimit is a token bucket enforced per PoP.
type EdgeRateLimit struct {
	RequestsPerSecond int    `json:"requests_per_second"`
	Burst             int    `json:"burst"`
	Key               string `json:"key"`
}

// EdgeJWTClaim is a required claim a JWT must carry to be accepted.
type EdgeJWTClaim struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EdgeJWTAuth validates inbound JWTs on a set of paths at the edge. It carries
// no secret material and is echoed verbatim on the settings response.
type EdgeJWTAuth struct {
	Enabled             bool           `json:"enabled"`
	Paths               []string       `json:"paths,omitempty"`
	JWKSURL             string         `json:"jwks_url,omitempty"`
	PublicKeys          []string       `json:"public_keys,omitempty"`
	Issuer              string         `json:"issuer,omitempty"`
	Audiences           []string       `json:"audiences,omitempty"`
	RequiredClaims      []EdgeJWTClaim `json:"required_claims,omitempty"`
	ForwardClaimsHeader string         `json:"forward_claims_header,omitempty"`
}

// EdgeSignedURLs enforces signed-URL access on a set of paths. SecretName is a
// reference to a stored secret by name only; the secret value never crosses the
// API. The same shape is echoed on the response (nothing secret is stored).
type EdgeSignedURLs struct {
	Enabled        bool     `json:"enabled"`
	Paths          []string `json:"paths,omitempty"`
	SecretName     string   `json:"secret_name,omitempty"`
	TTLSeconds     int      `json:"ttl_seconds,omitempty"`
	SignatureParam string   `json:"signature_param,omitempty"`
	ExpiresParam   string   `json:"expires_param,omitempty"`
}

// EdgeAPIKeyRequest is one inbound API key on the settings request. Key is the
// PLAINTEXT key the controller hashes and discards; it is write-only and never
// echoed. RateTier is an optional per-key rate limit.
type EdgeAPIKeyRequest struct {
	Name     string         `json:"name"`
	Key      string         `json:"key,omitempty"`
	RateTier *EdgeRateLimit `json:"rate_tier,omitempty"`
}

// EdgeAPIKeyAuthRequest enables API-key authentication on a set of paths at the
// edge. Keys carry plaintext key material that the controller hashes and
// discards; the stored document only ever carries the resulting hashes.
type EdgeAPIKeyAuthRequest struct {
	Enabled     bool                `json:"enabled"`
	Paths       []string            `json:"paths,omitempty"`
	KeyLocation string              `json:"key_location,omitempty"`
	KeyName     string              `json:"key_name,omitempty"`
	Keys        []EdgeAPIKeyRequest `json:"keys,omitempty"`
}

// EdgeAPIKeyView is the non-secret view of one inbound API key returned on the
// settings response: the key name and its optional rate tier, never the hash or
// the plaintext.
type EdgeAPIKeyView struct {
	Name     string         `json:"name"`
	RateTier *EdgeRateLimit `json:"rate_tier,omitempty"`
}

// EdgeAPIKeyAuthView is the non-secret view of the API-key authentication
// setting returned on the settings response. It carries no key material.
type EdgeAPIKeyAuthView struct {
	Enabled     bool             `json:"enabled"`
	Paths       []string         `json:"paths,omitempty"`
	KeyLocation string           `json:"key_location,omitempty"`
	KeyName     string           `json:"key_name,omitempty"`
	Keys        []EdgeAPIKeyView `json:"keys,omitempty"`
}

// EdgeWAFExclusion suppresses a WAF rule for a request target. At least one of
// RuleID or Target is set.
type EdgeWAFExclusion struct {
	RuleID int    `json:"rule_id,omitempty"`
	Target string `json:"target,omitempty"`
}

// EdgeDDoSProfile sets per-IP connection and request ceilings at the edge.
type EdgeDDoSProfile struct {
	Enabled               bool `json:"enabled"`
	PerIPRequestsPerSecond int `json:"per_ip_requests_per_second,omitempty"`
	PerIPBurst            int  `json:"per_ip_burst,omitempty"`
	PerIPConnCap         int  `json:"per_ip_conn_cap,omitempty"`
}

// EdgeBotManagement classifies and acts on automated traffic at the edge.
type EdgeBotManagement struct {
	Enabled            bool   `json:"enabled"`
	Action             string `json:"action,omitempty"`
	KnownBadBots       bool   `json:"known_bad_bots,omitempty"`
	RateBasedHeuristic bool   `json:"rate_based_heuristic,omitempty"`
}

// EdgeATOProtection guards authentication paths against account-takeover
// brute-forcing at the edge.
type EdgeATOProtection struct {
	Enabled                    bool     `json:"enabled"`
	AuthPaths                  []string `json:"auth_paths,omitempty"`
	FailureStatusCodes         []int    `json:"failure_status_codes,omitempty"`
	PerIPThresholdPerMin       int      `json:"per_ip_threshold_per_min,omitempty"`
	PerUsernameThresholdPerMin int      `json:"per_username_threshold_per_min,omitempty"`
	UsernameField              string   `json:"username_field,omitempty"`
	Action                     string   `json:"action,omitempty"`
}

// EdgeSettingsRequest holds the customer-tunable edge settings.
type EdgeSettingsRequest struct {
	CacheRules []EdgeCacheRule `json:"cache_rules,omitempty"`
	RateLimit  *EdgeRateLimit  `json:"rate_limit,omitempty"`
	WAFMode    *string         `json:"waf_mode,omitempty"`
	// Access / auth.
	JWTAuth    *EdgeJWTAuth           `json:"jwt_auth,omitempty"`
	SignedURLs *EdgeSignedURLs        `json:"signed_urls,omitempty"`
	APIKeyAuth *EdgeAPIKeyAuthRequest `json:"api_key_auth,omitempty"`
	// Security hardening.
	WAFParanoiaLevel int                `json:"waf_paranoia_level,omitempty"`
	WAFRuleExclusions []EdgeWAFExclusion `json:"waf_rule_exclusions,omitempty"`
	DDoSProfile      *EdgeDDoSProfile   `json:"ddos_profile,omitempty"`
	BotManagement    *EdgeBotManagement `json:"bot_management,omitempty"`
	ATOProtection    *EdgeATOProtection `json:"ato_protection,omitempty"`
}

// EdgeSettings is the response from an edge settings update or read.
type EdgeSettings struct {
	CacheRules    []EdgeCacheRule `json:"cache_rules,omitempty"`
	RateLimit     *EdgeRateLimit  `json:"rate_limit,omitempty"`
	WAFMode       string          `json:"waf_mode"`
	ConfigVersion int64           `json:"config_version"`
	// Access / auth (api_key_auth and signed_urls are non-secret views).
	JWTAuth    *EdgeJWTAuth        `json:"jwt_auth,omitempty"`
	SignedURLs *EdgeSignedURLs     `json:"signed_urls,omitempty"`
	APIKeyAuth *EdgeAPIKeyAuthView `json:"api_key_auth,omitempty"`
	// Security hardening.
	WAFParanoiaLevel  int                `json:"waf_paranoia_level,omitempty"`
	WAFRuleExclusions []EdgeWAFExclusion `json:"waf_rule_exclusions,omitempty"`
	DDoSProfile       *EdgeDDoSProfile   `json:"ddos_profile,omitempty"`
	BotManagement     *EdgeBotManagement `json:"bot_management,omitempty"`
	ATOProtection     *EdgeATOProtection `json:"ato_protection,omitempty"`
}

// EdgeNode is a platform edge point-of-presence (PoP): a set of one or more
// edge VMs in a single zone. When node_count is 2 or more the PoP runs a primary
// that holds the serving floating IP plus one or more hot standbys. The edge
// state machine converges node_count toward target_node_count.
type EdgeNode struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Zone            string `json:"zone"`
	Plan            string `json:"plan,omitempty"`
	Status          string `json:"status"`
	NodeCount       int    `json:"node_count"`
	TargetNodeCount int    `json:"target_node_count"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

type createEdgeNodeRequest struct {
	Zone      string `json:"zone"`
	Plan      string `json:"plan,omitempty"`
	NodeCount int    `json:"node_count"`
}

type scaleEdgeNodeRequest struct {
	TargetNodeCount int `json:"target_node_count"`
}

type listEdgeNodesResponse struct {
	Nodes []EdgeNode `json:"nodes"`
}

// CreateEdgeNode provisions a new edge PoP in the given zone. nodeCount sets the
// initial desired VM count (also the target); plan is optional and, when empty,
// is left to the controller default.
func (c *edgeClient) CreateEdgeNode(ctx context.Context, zone, plan string, nodeCount int) (*EdgeNode, error) {
	resp, err := c.do(ctx, http.MethodPost, "/admin/edge/nodes", createEdgeNodeRequest{
		Zone:      zone,
		Plan:      plan,
		NodeCount: nodeCount,
	})
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var n EdgeNode
	if err := json.Unmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("edge: decode CreateEdgeNode response: %w", err)
	}
	return &n, nil
}

// ListEdgeNodes returns all edge PoPs in the fleet.
func (c *edgeClient) ListEdgeNodes(ctx context.Context) ([]EdgeNode, error) {
	resp, err := c.do(ctx, http.MethodGet, "/admin/edge/nodes", nil)
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var result listEdgeNodesResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("edge: decode ListEdgeNodes response: %w", err)
	}
	return result.Nodes, nil
}

// GetEdgeNode finds an edge PoP by ID within the fleet list. Returns nil when no
// PoP with that ID exists (treats it as deleted).
func (c *edgeClient) GetEdgeNode(ctx context.Context, id string) (*EdgeNode, error) {
	nodes, err := c.ListEdgeNodes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i], nil
		}
	}
	return nil, nil
}

// ScaleEdgeNode sets the desired VM count for an edge PoP. The edge state machine
// converges node_count toward target_node_count asynchronously.
func (c *edgeClient) ScaleEdgeNode(ctx context.Context, id string, targetNodeCount int) (*EdgeNode, error) {
	resp, err := c.do(ctx, http.MethodPatch, "/admin/edge/nodes/"+id, scaleEdgeNodeRequest{
		TargetNodeCount: targetNodeCount,
	})
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var n EdgeNode
	if err := json.Unmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("edge: decode ScaleEdgeNode response: %w", err)
	}
	return &n, nil
}

// DeleteEdgeNode deprovisions an edge PoP. A 404 is treated as success (idempotent).
func (c *edgeClient) DeleteEdgeNode(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/admin/edge/nodes/"+id, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil
	}
	_, err = c.checkResp(resp)
	return err
}

// EdgeStatus is the edge overview for an app service.
type EdgeStatus struct {
	EdgeEnabled   bool          `json:"edge_enabled"`
	CNAMETarget   string        `json:"cname_target,omitempty"`
	ConfigVersion int64         `json:"config_version"`
	Settings      *EdgeSettings `json:"settings,omitempty"`
}

// GetEdgeStatus returns the edge status (and embedded settings) for an app service.
func (c *edgeClient) GetEdgeStatus(ctx context.Context, appServiceID string) (*EdgeStatus, error) {
	resp, err := c.do(ctx, http.MethodGet, "/app-services/"+appServiceID+"/edge", nil)
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var status EdgeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("edge: decode GetEdgeStatus response: %w", err)
	}
	return &status, nil
}

// UpdateEdgeSettings replaces the edge settings for an app service.
func (c *edgeClient) UpdateEdgeSettings(ctx context.Context, appServiceID string, req EdgeSettingsRequest) (*EdgeSettings, error) {
	resp, err := c.do(ctx, http.MethodPut, "/app-services/"+appServiceID+"/edge/settings", req)
	if err != nil {
		return nil, err
	}
	data, err := c.checkResp(resp)
	if err != nil {
		return nil, err
	}
	var settings EdgeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("edge: decode UpdateEdgeSettings response: %w", err)
	}
	return &settings, nil
}
