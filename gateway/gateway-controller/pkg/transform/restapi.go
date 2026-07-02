/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package transform

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	commonconstants "github.com/wso2/api-platform/common/constants"
	versionutil "github.com/wso2/api-platform/common/version"
	api "github.com/wso2/api-platform/gateway/gateway-controller/pkg/api/management"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/config"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/models"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/utils"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/utils/clusterkey"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/utils/upstreamref"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/xds"
	policyv1alpha "github.com/wso2/api-platform/sdk/core/policy/v1alpha2"
	policyenginev1 "github.com/wso2/api-platform/sdk/core/policyengine"
)

// RestAPITransformer transforms a StoredConfig (RestAPI kind) into a RuntimeDeployConfig.
type RestAPITransformer struct {
	routerConfig      *config.RouterConfig
	systemConfig      *config.Config
	policyDefinitions map[string]models.PolicyDefinition
	latestVersions    map[string]string // pre-computed policyName -> latest full semver
}

// NewRestAPITransformer creates a new RestAPITransformer.
func NewRestAPITransformer(
	routerConfig *config.RouterConfig,
	systemConfig *config.Config,
	policyDefinitions map[string]models.PolicyDefinition,
) *RestAPITransformer {
	return &RestAPITransformer{
		routerConfig:      routerConfig,
		systemConfig:      systemConfig,
		policyDefinitions: policyDefinitions,
		latestVersions:    config.BuildLatestVersionIndex(policyDefinitions),
	}
}

// Transform converts a StoredConfig with RestAPI configuration into a RuntimeDeployConfig.
// extractProjectID reads project ID from the annotation (preferred) then falls back to the
// deprecated bare label, logging a warning if only the label is present.
func extractProjectID(cfg *models.StoredConfig) string {
	if annotations := cfg.GetAnnotations(); annotations != nil {
		if pid, exists := (*annotations)[commonconstants.AnnotationProjectID]; exists {
			return pid
		}
	}
	if labels := cfg.GetLabels(); labels != nil {
		if pid, exists := (*labels)[commonconstants.DeprecatedLabelProjectID]; exists {
			slog.Warn("deprecated project-id label detected; migrate to annotation",
				"annotation", commonconstants.AnnotationProjectID,
				"api", cfg.Handle)
			return pid
		}
	}
	return ""
}

func (t *RestAPITransformer) Transform(cfg *models.StoredConfig) (*models.RuntimeDeployConfig, error) {
	restCfg, ok := cfg.Configuration.(api.RestAPI)
	if !ok {
		return nil, fmt.Errorf("configuration is not a RestAPI")
	}
	apiData := restCfg.Spec

	projectID := extractProjectID(cfg)

	rdc := &models.RuntimeDeployConfig{
		Metadata: models.Metadata{
			UUID:        cfg.UUID,
			Kind:        cfg.Kind,
			Handle:      cfg.Handle,
			Version:     apiData.Version,
			DisplayName: apiData.DisplayName,
			ProjectID:   projectID,
		},
		Context:             strings.ReplaceAll(apiData.Context, "$version", apiData.Version),
		PolicyChainResolver: "route-key",
		Routes:              make(map[string]*models.Route),
		PolicyChains:        make(map[string]*models.PolicyChain),
		UpstreamClusters:    make(map[string]*models.UpstreamCluster),
		SensitiveValues:     cfg.SensitiveValues,
	}

	// Collect validated API-level policies
	apiPolicies := t.collectAPIPolicies(apiData.Policies)

	// Determine effective vhosts
	effectiveMainVHost := t.routerConfig.VHosts.Main.Default
	effectiveSandboxVHost := t.routerConfig.VHosts.Sandbox.Default
	if apiData.Vhosts != nil {
		if strings.TrimSpace(apiData.Vhosts.Main) != "" {
			effectiveMainVHost = apiData.Vhosts.Main
		}
		if apiData.Vhosts.Sandbox != nil && strings.TrimSpace(*apiData.Vhosts.Sandbox) != "" {
			effectiveSandboxVHost = *apiData.Vhosts.Sandbox
		}
	}

	// Build main upstream cluster
	mainUpstream, err := t.addUpstreamCluster(rdc, "main", &apiData.Upstream.Main, apiData.UpstreamDefinitions)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve main upstream: %w", err)
	}

	// Check if dynamic cluster selection should be used
	useClusterHeader := apiData.UpstreamDefinitions != nil && len(*apiData.UpstreamDefinitions) > 0
	defaultCluster := ""
	if useClusterHeader {
		defaultCluster = mainUpstream.EnvoyClusterName
	}

	// Determine auto host rewrite for main upstream
	mainAutoHostRewrite := true
	if apiData.Upstream.Main.HostRewrite != nil && *apiData.Upstream.Main.HostRewrite == api.Manual {
		mainAutoHostRewrite = false
	}

	// Determine vhosts to create routes for.
	// Sandbox is active when a sandbox upstream is configured via either url or ref.
	apiSandboxHasContent := apiData.Upstream.Sandbox != nil &&
		((apiData.Upstream.Sandbox.Url != nil && strings.TrimSpace(*apiData.Upstream.Sandbox.Url) != "") ||
			(apiData.Upstream.Sandbox.Ref != nil && strings.TrimSpace(*apiData.Upstream.Sandbox.Ref) != ""))
	hasSandbox := apiSandboxHasContent

	// Also active if any operation defines a per-op sandbox upstream.
	if !hasSandbox {
		for _, op := range apiData.Operations {
			if op.Upstream != nil && op.Upstream.Sandbox != nil {
				sb := op.Upstream.Sandbox
				if strings.TrimSpace(sb.Ref) != "" {
					hasSandbox = true
					break
				}
			}
		}
	}

	// Guard: sandbox and main vhosts must differ, otherwise sandbox routes would
	// overwrite main routes (same route key) and the sandbox patch would leave only
	// a sandbox-cluster route with no main-cluster route at all.
	if hasSandbox && effectiveMainVHost == effectiveSandboxVHost {
		return nil, fmt.Errorf("sandbox upstream is configured but resolves to the same vhost %q as the main upstream; configure distinct vhosts to avoid route conflicts", effectiveMainVHost)
	}

	// Build routes and policy chains for each operation
	for _, op := range apiData.Operations {
		mainVhostClusterKey := mainUpstream.ClusterKey
		mainVhostUseClusterHeader := useClusterHeader
		mainVhostDefaultCluster := defaultCluster
		mainVhostAutoHostRewrite := mainAutoHostRewrite

		sandboxVhostClusterKey := mainUpstream.ClusterKey
		sandboxVhostUseClusterHeader := useClusterHeader
		sandboxVhostDefaultCluster := defaultCluster
		// Per-op sandbox routes carry no HostRewrite; inherit the API-level sandbox
		// setting when present, else the main setting (matches the xDS path).
		sandboxVhostAutoHostRewrite := mainAutoHostRewrite
		if apiSandboxHasContent {
			sandboxVhostAutoHostRewrite = true
			if apiData.Upstream.Sandbox.HostRewrite != nil && *apiData.Upstream.Sandbox.HostRewrite == api.Manual {
				sandboxVhostAutoHostRewrite = false
			}
		}

		if op.Upstream != nil {
			if op.Upstream.Main != nil {
				defClusterKey, err := perOpDefinitionClusterKey(cfg.Kind, cfg.UUID, op.Upstream.Main, apiData.UpstreamDefinitions)
				if err != nil {
					return nil, fmt.Errorf("per-op main upstream for %s %s: %w", string(op.Method), op.Path, err)
				}
				// Reuse the referenced definition's cluster as the cluster_header default
				// so a dynamic-endpoint policy can still steer this operation.
				mainVhostClusterKey = defClusterKey
				mainVhostUseClusterHeader = true
				mainVhostDefaultCluster = defClusterKey
				// AutoHostRewrite inherits API-level setting; per-op target is ref-only with no HostRewrite field.
			}
			if op.Upstream.Sandbox != nil {
				defClusterKey, err := perOpDefinitionClusterKey(cfg.Kind, cfg.UUID, op.Upstream.Sandbox, apiData.UpstreamDefinitions)
				if err != nil {
					return nil, fmt.Errorf("per-op sandbox upstream for %s %s: %w", string(op.Method), op.Path, err)
				}
				// Keep cluster_header ON for the sandbox per-op route so a sandbox
				// dynamic-endpoint policy can override the per-op default.
				sandboxVhostClusterKey = defClusterKey
				sandboxVhostUseClusterHeader = true
				sandboxVhostDefaultCluster = defClusterKey
				// AutoHostRewrite inherits API-level setting; per-op target is ref-only with no HostRewrite field.
			}
		}

		vhosts := []string{effectiveMainVHost}
		// Add the sandbox vhost only when this op has sandbox config (API-level
		// fallback or a per-op override); otherwise it would route to the main cluster.
		if apiSandboxHasContent || (op.Upstream != nil && op.Upstream.Sandbox != nil) {
			vhosts = append(vhosts, effectiveSandboxVHost)
		}

		for i, vhost := range vhosts {
			routeKey := xds.GenerateRouteName(string(op.Method), apiData.Context, apiData.Version, op.Path, vhost)

			clusterKey := mainVhostClusterKey
			vhostUseClusterHeader := mainVhostUseClusterHeader
			vhostDefaultCluster := mainVhostDefaultCluster
			autoHostRewrite := mainVhostAutoHostRewrite
			// vhosts[0] is always main and the sandbox vhost, when present, is appended
			// second; dispatch on position so equal vhost strings cannot misroute.
			if i > 0 {
				clusterKey = sandboxVhostClusterKey
				vhostUseClusterHeader = sandboxVhostUseClusterHeader
				vhostDefaultCluster = sandboxVhostDefaultCluster
				autoHostRewrite = sandboxVhostAutoHostRewrite
			}

			rdc.Routes[routeKey] = &models.Route{
				Method:          string(op.Method),
				Path:            xds.ConstructFullPath(apiData.Context, apiData.Version, op.Path),
				OperationPath:   op.Path,
				Vhost:           vhost,
				AutoHostRewrite: autoHostRewrite,
				Upstream: models.RouteUpstream{
					ClusterKey:       clusterKey,
					UseClusterHeader: vhostUseClusterHeader,
					DefaultCluster:   vhostDefaultCluster,
				},
			}

			// Build policy chain: API-level + operation-level + system policies
			chain := t.buildPolicyChain(apiPolicies, apiData.Policies, op.Policies)
			injected := utils.InjectSystemPolicies(chain, t.systemConfig, nil)
			rdc.PolicyChains[routeKey] = sdkChainToModel(injected)
		}
	}

	// Add upstream definition clusters for dynamic routing
	if apiData.UpstreamDefinitions != nil {
		for _, def := range *apiData.UpstreamDefinitions {
			if len(def.Upstreams) == 0 || def.Upstreams[0].Url == "" {
				continue
			}
			defClusterKey := clusterkey.DefinitionName(cfg.Kind, cfg.UUID, def.Name)
			parsedURL, err := url.Parse(def.Upstreams[0].Url)
			if err != nil {
				return nil, fmt.Errorf("invalid URL in upstream definition '%s': %w", def.Name, err)
			}
			port := ResolvePort(parsedURL)
			// Base path comes solely from the explicit basePath field; upstreamDefinitions
			// URLs are host[:port] only (a path in the URL is rejected during validation).
			basePath := "/"
			if def.BasePath != nil && *def.BasePath != "" {
				basePath = *def.BasePath
			}
			rdc.UpstreamClusters[defClusterKey] = &models.UpstreamCluster{
				Name:     def.Name,
				BasePath: basePath,
				Endpoints: []models.Endpoint{{
					Host: parsedURL.Hostname(),
					Port: port,
				}},
				TLS: &models.UpstreamTLS{Enabled: parsedURL.Scheme == "https"},
			}
		}
	}

	// Add sandbox upstream and update sandbox routes if present.
	// API-level sandbox is optional when per-op sandbox overrides exist.
	if apiSandboxHasContent {
		sbUpstream, err := t.addUpstreamCluster(rdc, "sandbox", apiData.Upstream.Sandbox, apiData.UpstreamDefinitions)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve sandbox upstream: %w", err)
		}

		sbAutoHostRewrite := true
		if apiData.Upstream.Sandbox.HostRewrite != nil && *apiData.Upstream.Sandbox.HostRewrite == api.Manual {
			sbAutoHostRewrite = false
		}

		// Update sandbox vhost routes to point to sandbox cluster, except ops with
		// their own per-op sandbox override (already wired in the main loop).
		for _, op := range apiData.Operations {
			if op.Upstream != nil && op.Upstream.Sandbox != nil {
				continue
			}
			routeKey := xds.GenerateRouteName(string(op.Method), apiData.Context, apiData.Version, op.Path, effectiveSandboxVHost)
			if r, exists := rdc.Routes[routeKey]; exists {
				r.Upstream.ClusterKey = sbUpstream.ClusterKey
				// Mirror main on sandbox routes: cluster_header lets a dynamic-endpoint policy
				// divert sandbox traffic, defaulting to the sandbox cluster when none does.
				r.Upstream.UseClusterHeader = useClusterHeader
				if useClusterHeader {
					r.Upstream.DefaultCluster = sbUpstream.EnvoyClusterName
				} else {
					r.Upstream.DefaultCluster = ""
				}
				r.AutoHostRewrite = sbAutoHostRewrite
			}
		}
	}

	return rdc, nil
}

// collectAPIPolicies validates and collects API-level policies into SDK format.
func (t *RestAPITransformer) collectAPIPolicies(policies *[]api.Policy) map[string]policyenginev1.PolicyInstance {
	result := make(map[string]policyenginev1.PolicyInstance)
	if policies == nil {
		return result
	}
	for _, p := range *policies {
		resolved, err := config.ResolvePolicyVersion(t.policyDefinitions, t.latestVersions, p.Name, p.Version)
		if err != nil {
			slog.Error("Failed to resolve policy version for API-level policy", "policy_name", p.Name, "error", err)
			continue
		}
		result[p.Name] = convertAPIPolicyToSDK(p, policyv1alpha.LevelAPI, versionutil.MajorVersion(resolved))
	}
	return result
}

// buildPolicyChain builds a merged list: API-level + operation-level policies (SDK format).
func (t *RestAPITransformer) buildPolicyChain(
	apiPolicies map[string]policyenginev1.PolicyInstance,
	specPolicies *[]api.Policy,
	opPolicies *[]api.Policy,
) []policyenginev1.PolicyInstance {
	var result []policyenginev1.PolicyInstance

	// API-level policies (in spec order, validated via apiPolicies map)
	if specPolicies != nil {
		for _, p := range *specPolicies {
			if v, ok := apiPolicies[p.Name]; ok {
				result = append(result, v)
			}
		}
	}

	// Operation-level policies
	if opPolicies != nil {
		for _, opPol := range *opPolicies {
			resolved, err := config.ResolvePolicyVersion(t.policyDefinitions, t.latestVersions, opPol.Name, opPol.Version)
			if err != nil {
				slog.Error("Failed to resolve operation-level policy version", "policy_name", opPol.Name, "error", err)
				continue
			}
			result = append(result, convertAPIPolicyToSDK(opPol, policyv1alpha.LevelRoute, versionutil.MajorVersion(resolved)))
		}
	}

	return result
}

// upstreamClusterResult holds the result of resolving and registering an upstream cluster.
type upstreamClusterResult struct {
	// ClusterKey is the internal key used in rdc.UpstreamClusters.
	ClusterKey string
	// EnvoyClusterName is the name Envoy knows the cluster by, used by the policy
	// engine for the x-target-upstream header. It is always set equal to ClusterKey.
	EnvoyClusterName string
	// BasePath is the URL path component of the upstream (e.g. "/anything/foo").
	BasePath string
}

// addUpstreamCluster resolves an upstream and adds it to the RuntimeDeployConfig.
func (t *RestAPITransformer) addUpstreamCluster(
	rdc *models.RuntimeDeployConfig,
	upstreamName string,
	up *api.Upstream,
	upstreamDefinitions *[]api.UpstreamDefinition,
) (*upstreamClusterResult, error) {
	rawURL, refBasePath, err := resolveUpstreamURL(upstreamName, up, upstreamDefinitions)
	if err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid %s upstream URL: %w", upstreamName, err)
	}
	if parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid %s upstream URL: must include host and http/https scheme", upstreamName)
	}

	port := ResolvePort(parsedURL)
	// Direct URLs carry their base path in the path; a ref takes its base path solely from
	// the definition's basePath field (upstreamDefinitions URLs are host[:port] only).
	basePath := parsedURL.Path
	if refBasePath != nil {
		basePath = *refBasePath
	}
	if basePath == "" {
		basePath = "/"
	}

	// URL-stable cluster name so a URL edit updates the same cluster instead of
	// renaming it. ClusterKey and EnvoyClusterName are intentionally identical.
	clusterKey := clusterkey.APILevelName(upstreamName, rdc.Metadata.UUID)

	rdc.UpstreamClusters[clusterKey] = &models.UpstreamCluster{
		BasePath: basePath,
		Endpoints: []models.Endpoint{{
			Host: parsedURL.Hostname(),
			Port: port,
		}},
		TLS: &models.UpstreamTLS{Enabled: parsedURL.Scheme == "https"},
	}

	// ClusterKey and EnvoyClusterName must stay identical or the default upstream
	// path yields 503 NoRoute.
	return &upstreamClusterResult{
		ClusterKey:       clusterKey,
		EnvoyClusterName: clusterKey,
		BasePath:         basePath,
	}, nil
}

// perOpDefinitionClusterKey resolves a ref-only per-op target to the cluster key of
// the referenced upstreamDefinition, reusing that definition's cluster (and its
// basePath) instead of minting a per-op one.
func perOpDefinitionClusterKey(kind, uuid string, target *api.RestAPIOperationUpstreamTarget, upstreamDefinitions *[]api.UpstreamDefinition) (string, error) {
	def, err := upstreamref.FindByName(target.Ref, upstreamDefinitions)
	if err != nil {
		return "", err
	}
	if len(def.Upstreams) == 0 || def.Upstreams[0].Url == "" {
		return "", fmt.Errorf("upstream definition '%s' has no URLs", strings.TrimSpace(target.Ref))
	}
	return clusterkey.DefinitionName(kind, uuid, def.Name), nil
}

// resolveUpstreamURL resolves the URL from an upstream (direct URL or ref). For a ref it
// also returns the referenced definition's base path (from basePath, never the URL); for a
// direct URL the returned base-path pointer is nil, signalling the caller to use the URL path.
func resolveUpstreamURL(name string, up *api.Upstream, defs *[]api.UpstreamDefinition) (string, *string, error) {
	if up.Url != nil && strings.TrimSpace(*up.Url) != "" {
		return strings.TrimSpace(*up.Url), nil, nil
	}
	if up.Ref != nil && strings.TrimSpace(*up.Ref) != "" {
		refName := strings.TrimSpace(*up.Ref)
		// Resolve via the shared upstreamref helper and return the definition's
		// basePath so the caller rewrites the upstream path correctly.
		def, err := upstreamref.FindByName(refName, defs)
		if err != nil {
			return "", nil, err
		}
		if len(def.Upstreams) == 0 || def.Upstreams[0].Url == "" {
			return "", nil, fmt.Errorf("upstream definition '%s' has no URLs", refName)
		}
		basePath := ""
		if def.BasePath != nil {
			basePath = *def.BasePath
		}
		return def.Upstreams[0].Url, &basePath, nil
	}
	return "", nil, fmt.Errorf("%s upstream has no URL or ref", name)
}

// ResolvePort returns the port from a URL, defaulting to 80/443.
func ResolvePort(u *url.URL) int {
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err == nil {
			return port
		}
	}
	if u.Scheme == "https" {
		return 443
	}
	return 80
}

// convertAPIPolicyToSDK converts an api.Policy to policyenginev1.PolicyInstance.
func convertAPIPolicyToSDK(p api.Policy, attachedTo policyv1alpha.Level, resolvedVersion string) policyenginev1.PolicyInstance {
	paramsMap := make(map[string]interface{})
	if p.Params != nil {
		for k, v := range *p.Params {
			paramsMap[k] = v
		}
	}
	if attachedTo != "" {
		paramsMap["attachedTo"] = string(attachedTo)
	}

	return policyenginev1.PolicyInstance{
		Name:               p.Name,
		Version:            resolvedVersion,
		Enabled:            true,
		ExecutionCondition: p.ExecutionCondition,
		Parameters:         paramsMap,
	}
}

// sdkChainToModel converts a slice of SDK PolicyInstance to a models.PolicyChain.
func sdkChainToModel(instances []policyenginev1.PolicyInstance) *models.PolicyChain {
	chain := &models.PolicyChain{
		Policies: make([]models.Policy, 0, len(instances)),
	}
	for _, inst := range instances {
		chain.Policies = append(chain.Policies, models.Policy{
			Name:               inst.Name,
			Version:            inst.Version,
			Params:             inst.Parameters,
			ExecutionCondition: inst.ExecutionCondition,
		})
	}
	return chain
}
