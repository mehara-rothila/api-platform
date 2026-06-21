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
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api "github.com/wso2/api-platform/gateway/gateway-controller/pkg/api/management"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/config"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/models"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/utils/clusterkey"
)

// ptrStr is a helper to get a pointer to a string literal.
func ptrStr(s string) *string { return &s }

// testRouterCfg returns a minimal RouterConfig for transformer tests.
func testRouterCfg() *config.RouterConfig {
	return &config.RouterConfig{
		GatewayHost: "gw.local",
		VHosts: config.VHostsConfig{
			Main:    config.VHostEntry{Default: "main.local"},
			Sandbox: config.VHostEntry{Default: "sandbox.local"},
		},
	}
}

// makeRestAPIStoredConfig builds a minimal RestAPI StoredConfig for transformer tests.
func makeRestAPIStoredConfig(apiPolicies []api.Policy, opPolicies []api.Policy) *models.StoredConfig {
	var specPolicies *[]api.Policy
	if apiPolicies != nil {
		specPolicies = &apiPolicies
	}

	var opPols *[]api.Policy
	if opPolicies != nil {
		opPols = &opPolicies
	}

	apiData := api.APIConfigData{
		DisplayName: "Test API",
		Context:     "/test",
		Version:     "1.0.0",
		Operations: []api.Operation{
			{Method: "GET", Path: "/hello", Policies: opPols},
		},
		Policies: specPolicies,
		Upstream: struct {
			Main    api.Upstream  `json:"main" yaml:"main"`
			Sandbox *api.Upstream `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
		}{
			Main: api.Upstream{Url: ptrStr("http://backend:8080")},
		},
	}

	restAPI := api.RestAPI{
		Kind:     api.RestAPIKindRestApi,
		Metadata: api.Metadata{Name: "test-api"},
		Spec:     apiData,
	}

	return &models.StoredConfig{
		UUID:          "test-api",
		Kind:          string(api.RestAPIKindRestApi),
		Configuration: restAPI,
	}
}

// findPolicyInChain returns true if any policy chain for the given route key
// contains a policy with the given name.
func findPolicyInChain(rdc *models.RuntimeDeployConfig, routeKey, policyName string) bool {
	chain, ok := rdc.PolicyChains[routeKey]
	if !ok {
		return false
	}
	for _, p := range chain.Policies {
		if p.Name == policyName {
			return true
		}
	}
	return false
}

// TestRestAPITransformer_APILevelEmptyVersionResolvesToLatest verifies that an API-level
// policy with an empty version is resolved via the pre-computed latestVersions index
// and included in the policy chain for every operation.
func TestRestAPITransformer_APILevelEmptyVersionResolvesToLatest(t *testing.T) {
	defs := map[string]models.PolicyDefinition{
		"header-mutate|v1.0.0": {Name: "header-mutate", Version: "v1.0.0"},
		"header-mutate|v2.0.0": {Name: "header-mutate", Version: "v2.0.0"},
	}

	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)
	cfg := makeRestAPIStoredConfig(
		[]api.Policy{{Name: "header-mutate", Version: ""}}, // empty version
		nil,
	)

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	// Route key for GET /hello on the main vhost
	routeKey := "GET|/test/hello|main.local"
	assert.True(t, findPolicyInChain(rdc, routeKey, "header-mutate"),
		"expected header-mutate to be in policy chain when empty version is specified")
}

// TestRestAPITransformer_OperationLevelEmptyVersionResolvesToLatest verifies that an
// operation-level policy with an empty version is resolved via the pre-computed index
// and included in the policy chain.
func TestRestAPITransformer_OperationLevelEmptyVersionResolvesToLatest(t *testing.T) {
	defs := map[string]models.PolicyDefinition{
		"rate-limit|v1.0.0": {Name: "rate-limit", Version: "v1.0.0"},
	}

	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)
	cfg := makeRestAPIStoredConfig(
		nil,
		[]api.Policy{{Name: "rate-limit", Version: ""}}, // empty version at op level
	)

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	routeKey := "GET|/test/hello|main.local"
	assert.True(t, findPolicyInChain(rdc, routeKey, "rate-limit"),
		"expected rate-limit to be in policy chain when empty version is specified at operation level")
}

// TestRestAPITransformer_UnknownPolicySkipped verifies that a policy not present in
// the definitions is silently excluded from the policy chain without causing an error.
func TestRestAPITransformer_UnknownPolicySkipped(t *testing.T) {
	defs := map[string]models.PolicyDefinition{} // empty - policy won't resolve

	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)
	cfg := makeRestAPIStoredConfig(
		[]api.Policy{{Name: "unknown-policy", Version: ""}},
		nil,
	)

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	routeKey := "GET|/test/hello|main.local"
	assert.False(t, findPolicyInChain(rdc, routeKey, "unknown-policy"),
		"expected unknown-policy to be excluded from the policy chain")
}

// TestRestAPITransformer_LatestVersionIndexBuiltOnConstruction verifies that the
// pre-computed index is populated when the transformer is constructed, meaning
// repeated Transform calls resolve without re-scanning definitions.
func TestRestAPITransformer_LatestVersionIndexBuiltOnConstruction(t *testing.T) {
	defs := map[string]models.PolicyDefinition{
		"auth|v1.0.0": {Name: "auth", Version: "v1.0.0"},
		"auth|v3.0.0": {Name: "auth", Version: "v3.0.0"},
		"auth|v2.0.0": {Name: "auth", Version: "v2.0.0"},
	}

	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)

	// Verify the pre-computed index has the correct latest version.
	assert.Equal(t, "v3.0.0", transformer.latestVersions["auth"],
		"pre-computed index should hold the highest semver for each policy")

	// Verify Transform works correctly using the index (empty version resolves to v3.0.0).
	cfg := makeRestAPIStoredConfig(
		[]api.Policy{{Name: "auth", Version: ""}},
		nil,
	)
	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	routeKey := "GET|/test/hello|main.local"
	assert.True(t, findPolicyInChain(rdc, routeKey, "auth"))
}

// TestRestAPITransformer_EmptyVersionUsesResolvedVersionInChain verifies that the
// resolved full semver (not the original empty string) is stored in the policy chain,
// so the policy engine can match it to the correct policy definition.
func TestRestAPITransformer_EmptyVersionUsesResolvedVersionInChain(t *testing.T) {
	defs := map[string]models.PolicyDefinition{
		"header-mutate|v2.0.0": {Name: "header-mutate", Version: "v2.0.0"},
	}

	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)
	cfg := makeRestAPIStoredConfig(
		[]api.Policy{{Name: "header-mutate", Version: ""}},
		nil,
	)

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	routeKey := "GET|/test/hello|main.local"
	chain, ok := rdc.PolicyChains[routeKey]
	require.True(t, ok)
	require.Len(t, chain.Policies, 1)
	assert.Equal(t, "v2", chain.Policies[0].Version,
		"resolved major version should be stored in the chain, not the original empty string")
}

// TestResolveUpstreamURL verifies URL resolution from direct URL, ref, or missing config.
func TestResolveUpstreamURL(t *testing.T) {
	refName := "my-def"
	defURL := "http://upstream-from-def:9090"

	defs := &[]api.UpstreamDefinition{
		{
			Name: refName,
			Upstreams: []struct {
				Url    string `json:"url" yaml:"url"`
				Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
			}{
				{Url: defURL},
			},
		},
	}

	t.Run("direct URL", func(t *testing.T) {
		u := "http://direct:8080"
		up := &api.Upstream{Url: &u}
		got, _, err := resolveUpstreamURL("main", up, nil)
		require.NoError(t, err)
		assert.Equal(t, u, got)
	})

	t.Run("ref to existing definition", func(t *testing.T) {
		up := &api.Upstream{Ref: &refName}
		got, _, err := resolveUpstreamURL("main", up, defs)
		require.NoError(t, err)
		assert.Equal(t, defURL, got)
	})

	t.Run("ref but no definitions provided", func(t *testing.T) {
		up := &api.Upstream{Ref: &refName}
		_, _, err := resolveUpstreamURL("main", up, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), refName)
	})

	t.Run("ref to unknown definition", func(t *testing.T) {
		unknownRef := "unknown-def"
		up := &api.Upstream{Ref: &unknownRef}
		_, _, err := resolveUpstreamURL("main", up, defs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("neither URL nor ref", func(t *testing.T) {
		up := &api.Upstream{}
		_, _, err := resolveUpstreamURL("main", up, defs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no URL or ref")
	})

	t.Run("whitespace-only URL treated as missing", func(t *testing.T) {
		blank := "   "
		up := &api.Upstream{Url: &blank}
		_, _, err := resolveUpstreamURL("main", up, nil)
		require.Error(t, err)
	})

	t.Run("def with no upstreams", func(t *testing.T) {
		emptyDefs := &[]api.UpstreamDefinition{
			{
				Name: "empty-def",
				Upstreams: []struct {
					Url    string `json:"url" yaml:"url"`
					Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
				}{},
			},
		}
		emptyRef := "empty-def"
		up := &api.Upstream{Ref: &emptyRef}
		_, _, err := resolveUpstreamURL("main", up, emptyDefs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no URLs")
	})
}

// makeRestAPIWithOps builds a RestAPI StoredConfig with caller-supplied operations,
// both API-level main and sandbox upstreams configured, and a set of common
// upstreamDefinitions that per-op tests can reference by name.
func makeRestAPIWithOps(ops []api.Operation) *models.StoredConfig {
	defs := []api.UpstreamDefinition{
		{Name: "user-svc-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://user-svc:8080"}}},
		{Name: "user-svc-test-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://user-svc-test:8080"}}},
		{Name: "shared-svc-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://shared-svc:8080"}}},
		{Name: "same-svc-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://same-svc:8080"}}},
		{Name: "user-svc-cluster-v2", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://user-svc:9090"}}},
		{Name: "per-op-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://per-op-main:9090"}}},
	}
	apiData := api.APIConfigData{
		DisplayName:         "Test API",
		Context:             "/test",
		Version:             "1.0.0",
		Operations:          ops,
		UpstreamDefinitions: &defs,
		Upstream: struct {
			Main    api.Upstream  `json:"main" yaml:"main"`
			Sandbox *api.Upstream `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
		}{
			Main:    api.Upstream{Url: ptrStr("http://api-main:8080")},
			Sandbox: &api.Upstream{Url: ptrStr("http://api-sandbox:8080")},
		},
	}
	restAPI := api.RestAPI{
		Kind:     api.RestAPIKindRestApi,
		Metadata: api.Metadata{Name: "test-api"},
		Spec:     apiData,
	}
	return &models.StoredConfig{
		UUID:          "test-api",
		Kind:          string(api.RestAPIKindRestApi),
		Configuration: restAPI,
	}
}

// TestRestAPITransformer_PerOpMainOverridesMainVhost asserts that a main-only override
// causes the main vhost route to use the definition cluster while the sandbox vhost route
// falls back to the API-level sandbox cluster.
func TestRestAPITransformer_PerOpMainOverridesMainVhost(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.RestAPIOperationUpstream{
				Main: &api.RestAPIOperationUpstreamTarget{Ref: "user-svc-cluster"},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	assert.True(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "upstream_"),
		"main vhost should use definition cluster, got %q", mainRoute.Upstream.ClusterKey)
	// Per-op main is dynamic: cluster_header ON with the definition cluster as the
	// default, so a dynamic-endpoint policy can still steer it while a no-policy
	// request falls back to the per-op ref.
	assert.True(t, mainRoute.Upstream.UseClusterHeader,
		"per-op main route should use cluster_header so policies can override")
	assert.Equal(t, mainRoute.Upstream.ClusterKey, mainRoute.Upstream.DefaultCluster,
		"per-op main DefaultCluster must be the definition cluster key")

	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, sandboxRoute)
	assert.False(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "upstream_"),
		"sandbox vhost should fall back to API sandbox, got %q", sandboxRoute.Upstream.ClusterKey)
}

// TestRestAPITransformer_PerOpSandboxOverridesSandboxVhost asserts that a sandbox-only override
// causes the main vhost to fall back to the API main while the sandbox vhost uses the definition cluster.
func TestRestAPITransformer_PerOpSandboxOverridesSandboxVhost(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.RestAPIOperationUpstream{
				Sandbox: &api.RestAPIOperationUpstreamTarget{Ref: "user-svc-test-cluster"},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	assert.False(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "upstream_"),
		"main vhost should fall back to API main, got %q", mainRoute.Upstream.ClusterKey)

	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, sandboxRoute)
	assert.True(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "upstream_"),
		"sandbox vhost should use definition cluster, got %q", sandboxRoute.Upstream.ClusterKey)
}

// TestRestAPITransformer_PerOpBothOverrideBothVhosts asserts that both vhosts get distinct
// definition clusters when main and sandbox are overridden.
func TestRestAPITransformer_PerOpBothOverrideBothVhosts(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.RestAPIOperationUpstream{
				Main:    &api.RestAPIOperationUpstreamTarget{Ref: "user-svc-cluster"},
				Sandbox: &api.RestAPIOperationUpstreamTarget{Ref: "user-svc-test-cluster"},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, mainRoute)
	require.NotNil(t, sandboxRoute)

	assert.True(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "upstream_"))
	assert.True(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "upstream_"))
	assert.NotEqual(t, mainRoute.Upstream.ClusterKey, sandboxRoute.Upstream.ClusterKey,
		"main and sandbox per-op vhosts must produce distinct cluster keys (definition names differ)")
}

// TestRestAPITransformer_NoPerOpUsesAPILevelClusters - regression - without per-op
// upstream the routes still use the API-level main/sandbox clusters.
func TestRestAPITransformer_NoPerOpUsesAPILevelClusters(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users"},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, mainRoute)
	require.NotNil(t, sandboxRoute)
	assert.False(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "upstream_"))
	assert.False(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "upstream_"))
}

// TestRestAPITransformer_TwoOpsSameRefReuseOneCluster verifies the core reuse
// property: two operations referencing the SAME upstreamDefinition reuse exactly
// ONE definition cluster (no definition clusters), and both routes point at it.
func TestRestAPITransformer_TwoOpsSameRefReuseOneCluster(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users", Upstream: &api.RestAPIOperationUpstream{Main: &api.RestAPIOperationUpstreamTarget{Ref: "shared-svc"}}},
		{Method: "POST", Path: "/users", Upstream: &api.RestAPIOperationUpstream{Main: &api.RestAPIOperationUpstreamTarget{Ref: "shared-svc"}}},
	})
	spec := cfg.Configuration.(api.RestAPI)
	spec.Spec.UpstreamDefinitions = &[]api.UpstreamDefinition{
		{
			Name: "shared-svc",
			Upstreams: []struct {
				Url    string `json:"url" yaml:"url"`
				Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
			}{
				{Url: "http://shared-svc:8080"},
			},
		},
	}
	cfg.Configuration = spec

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	getRoute := rdc.Routes["GET|/test/users|main.local"]
	postRoute := rdc.Routes["POST|/test/users|main.local"]
	require.NotNil(t, getRoute, "GET route must exist")
	require.NotNil(t, postRoute, "POST route must exist")

	// Both ops reuse the SAME definition cluster (no definition clusters).
	assert.Equal(t, getRoute.Upstream.ClusterKey, postRoute.Upstream.ClusterKey,
		"two ops sharing a ref must reuse the same definition cluster")
	assert.True(t, strings.HasPrefix(getRoute.Upstream.ClusterKey, "upstream_"),
		"per-op route must reuse the upstream_<def> definition cluster, got %q", getRoute.Upstream.ClusterKey)

	// Exactly ONE cluster registered for shared-svc; zero op_ clusters.
	shared := 0
	for k := range rdc.UpstreamClusters {
		assert.False(t, strings.HasPrefix(k, "op_"), "no per-op (op_) clusters may be minted, got %q", k)
		if strings.Contains(k, "shared-svc") {
			shared++
		}
	}
	assert.Equal(t, 1, shared, "shared-svc must produce exactly one reused definition cluster")
}

// TestRestAPITransformer_PerOpClusterIsolatedAcrossAPIs asserts that two APIs with the
// same operation referencing the same definition produce different definition cluster
// keys because the API ID is part of the cluster name.
func TestRestAPITransformer_PerOpClusterIsolatedAcrossAPIs(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	cfgA := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.RestAPIOperationUpstream{
				Main: &api.RestAPIOperationUpstreamTarget{Ref: "shared-svc-cluster"},
			},
		},
	})
	cfgA.UUID = "api-aaa"

	cfgB := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.RestAPIOperationUpstream{
				Main: &api.RestAPIOperationUpstreamTarget{Ref: "shared-svc-cluster"},
			},
		},
	})
	cfgB.UUID = "api-bbb"

	rdcA, err := transformer.Transform(cfgA)
	require.NoError(t, err)
	rdcB, err := transformer.Transform(cfgB)
	require.NoError(t, err)

	var keyA, keyB string
	for k := range rdcA.UpstreamClusters {
		if strings.HasPrefix(k, "upstream_") {
			keyA = k
		}
	}
	for k := range rdcB.UpstreamClusters {
		if strings.HasPrefix(k, "upstream_") {
			keyB = k
		}
	}

	require.NotEmpty(t, keyA)
	require.NotEmpty(t, keyB)
	assert.NotEqual(t, keyA, keyB, "same URL across different APIs must produce different definition cluster keys")
}

// TestRestAPITransformer_PerOpSandboxWithoutAPILevelSandbox - guard regression.
// API-level Sandbox is nil, but one op declares a per-op sandbox upstream. The
// sandbox vhost must be created only for that op; ops without per-op sandbox
// must NOT get a sandbox route (otherwise they'd silently route to the main
// cluster on the sandbox vhost).
func TestRestAPITransformer_PerOpSandboxWithoutAPILevelSandbox(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	sbDefs := []api.UpstreamDefinition{
		{Name: "user-svc-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://user-svc-test:8080"}}},
	}
	apiData := api.APIConfigData{
		DisplayName:         "Test API",
		Context:             "/test",
		Version:             "1.0.0",
		UpstreamDefinitions: &sbDefs,
		Operations: []api.Operation{
			{
				Method: "GET", Path: "/users",
				Upstream: &api.RestAPIOperationUpstream{
					Sandbox: &api.RestAPIOperationUpstreamTarget{Ref: "user-svc-cluster"},
				},
			},
			{Method: "GET", Path: "/orders"},
		},
		Upstream: struct {
			Main    api.Upstream  `json:"main" yaml:"main"`
			Sandbox *api.Upstream `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
		}{
			Main:    api.Upstream{Url: ptrStr("http://api-main:8080")},
			Sandbox: nil,
		},
	}
	cfg := &models.StoredConfig{
		UUID: "test-api",
		Kind: string(api.RestAPIKindRestApi),
		Configuration: api.RestAPI{
			Kind:     api.RestAPIKindRestApi,
			Metadata: api.Metadata{Name: "test-api"},
			Spec:     apiData,
		},
	}

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	usersMain := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, usersMain, "op with per-op sandbox must still have a main route")
	assert.False(t, strings.HasPrefix(usersMain.Upstream.ClusterKey, "upstream_"),
		"main vhost should fall back to API main cluster, got %q", usersMain.Upstream.ClusterKey)

	usersSandbox := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, usersSandbox, "op with per-op sandbox must have a sandbox route")
	assert.True(t, strings.HasPrefix(usersSandbox.Upstream.ClusterKey, "upstream_"),
		"sandbox vhost should use definition cluster, got %q", usersSandbox.Upstream.ClusterKey)

	ordersMain := rdc.Routes["GET|/test/orders|main.local"]
	require.NotNil(t, ordersMain, "op without per-op upstream must have a main route")
	assert.False(t, strings.HasPrefix(ordersMain.Upstream.ClusterKey, "upstream_"))

	_, ordersHasSandbox := rdc.Routes["GET|/test/orders|sandbox.local"]
	assert.False(t, ordersHasSandbox,
		"op without per-op sandbox must NOT get a sandbox route when API-level sandbox is nil")
}

// TestRestAPITransformer_PerOpSandboxInheritsSandboxHostRewrite - a per-op sandbox
// override route carries no HostRewrite of its own, so it must inherit the API-level
// SANDBOX HostRewrite (not the API-level main). This guards the transform/xDS parity:
// the xDS path inherits the sandbox value, so the RDC path must too. With API-level
// main=auto and sandbox=manual, the per-op sandbox route must be manual (AutoHostRewrite=false).
func TestRestAPITransformer_PerOpSandboxInheritsSandboxHostRewrite(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	manual := api.Manual
	auto := api.Auto
	defs := []api.UpstreamDefinition{
		{Name: "op-sandbox-cluster", Upstreams: []struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{{Url: "http://op-sandbox:8080"}}},
	}
	apiData := api.APIConfigData{
		DisplayName:         "Test API",
		Context:             "/test",
		Version:             "1.0.0",
		UpstreamDefinitions: &defs,
		Operations: []api.Operation{
			{
				Method: "GET", Path: "/users",
				Upstream: &api.RestAPIOperationUpstream{
					Sandbox: &api.RestAPIOperationUpstreamTarget{Ref: "op-sandbox-cluster"},
				},
			},
		},
		Upstream: struct {
			Main    api.Upstream  `json:"main" yaml:"main"`
			Sandbox *api.Upstream `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
		}{
			Main:    api.Upstream{Url: ptrStr("http://api-main:8080"), HostRewrite: &auto},
			Sandbox: &api.Upstream{Url: ptrStr("http://api-sandbox:8080"), HostRewrite: &manual},
		},
	}
	cfg := &models.StoredConfig{
		UUID: "test-api",
		Kind: string(api.RestAPIKindRestApi),
		Configuration: api.RestAPI{
			Kind:     api.RestAPIKindRestApi,
			Metadata: api.Metadata{Name: "test-api"},
			Spec:     apiData,
		},
	}

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	usersSandbox := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, usersSandbox, "op with per-op sandbox must have a sandbox route")
	assert.True(t, strings.HasPrefix(usersSandbox.Upstream.ClusterKey, "upstream_"),
		"sandbox vhost should use definition cluster, got %q", usersSandbox.Upstream.ClusterKey)
	assert.False(t, usersSandbox.AutoHostRewrite,
		"per-op sandbox route must inherit API-level SANDBOX hostRewrite (manual), not main (auto)")

	usersMain := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, usersMain)
	assert.True(t, usersMain.AutoHostRewrite,
		"main route must keep API-level main hostRewrite (auto)")
}

// TestResolvePort checks port resolution with explicit, default-http and default-https.
func TestResolvePort(t *testing.T) {
	tests := []struct {
		rawURL   string
		expected int
	}{
		{"http://host:9090/path", 9090},
		{"http://host/path", 80},
		{"https://host/path", 443},
		{"https://host:8443/path", 8443},
	}

	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ResolvePort(u))
		})
	}
}

// TestRestAPITransformer_SandboxRouteClusterHeader pins the sandbox route's dynamic
// cluster selection: with upstreamDefinitions the sandbox route uses cluster_header
// routing (so a dynamic-endpoint policy can divert sandbox traffic) and defaults to the
// sandbox cluster; without them it stays a static sandbox route.
func TestRestAPITransformer_SandboxRouteClusterHeader(t *testing.T) {
	defs := map[string]models.PolicyDefinition{}
	const sandboxURL = "http://sandbox-backend:9080/sandbox"
	const sandboxRouteKey = "GET|/test/hello|sandbox.local"

	t.Run("without upstreamDefinitions the sandbox route is static", func(t *testing.T) {
		transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)
		cfg := makeRestAPIStoredConfig(nil, nil)
		restAPI := cfg.Configuration.(api.RestAPI)
		restAPI.Spec.Upstream.Sandbox = &api.Upstream{Url: ptrStr(sandboxURL)}
		cfg.Configuration = restAPI

		rdc, err := transformer.Transform(cfg)
		require.NoError(t, err)
		r, exists := rdc.Routes[sandboxRouteKey]
		require.True(t, exists, "sandbox route should exist")
		assert.False(t, r.Upstream.UseClusterHeader)
		assert.Equal(t, "", r.Upstream.DefaultCluster)
	})

	t.Run("with upstreamDefinitions the sandbox route uses cluster_header defaulting to the sandbox cluster", func(t *testing.T) {
		transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, defs)
		cfg := makeRestAPIStoredConfig(nil, nil)
		restAPI := cfg.Configuration.(api.RestAPI)
		restAPI.Spec.Upstream.Sandbox = &api.Upstream{Url: ptrStr(sandboxURL)}
		restAPI.Spec.UpstreamDefinitions = &[]api.UpstreamDefinition{{Name: "alt-upstream"}}
		cfg.Configuration = restAPI

		rdc, err := transformer.Transform(cfg)
		require.NoError(t, err)
		r, exists := rdc.Routes[sandboxRouteKey]
		require.True(t, exists, "sandbox route should exist")
		assert.True(t, r.Upstream.UseClusterHeader)
		assert.True(t, strings.HasPrefix(r.Upstream.DefaultCluster, "sandbox_"),
			"sandbox route must default to the URL-stable sandbox cluster (sandbox_<hash>), not main; got %q", r.Upstream.DefaultCluster)
	})
}

// TestRestAPITransformer_APILevelClusterNameShape asserts the URL-stable cluster
// naming contract for API-level main and sandbox upstreams:
//   - cluster names are "<env>_<24-hex>" derived from sha256(apiID), shared by main and sandbox
//   - ClusterKey and EnvoyClusterName are the SAME string (so the policy engine's
//     default_upstream_cluster metadata resolves to a real Envoy cluster)
func TestRestAPITransformer_APILevelClusterNameShape(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users"},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	// Expected name is hard-coded (sha256("test-api")[:12]), not computed via
	// clusterkey.APILevel, so a change to the hashing function is caught here.
	expectedMain := "main_2a28373e2cacc6ea903d8c7e"
	expectedSandbox := "sandbox_2a28373e2cacc6ea903d8c7e"

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute, "main route must exist")
	assert.Equal(t, expectedMain, mainRoute.Upstream.ClusterKey,
		"main cluster name should be <env>_<hash> derived from sha256(apiID)")

	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, sandboxRoute, "sandbox route must exist")
	assert.Equal(t, expectedSandbox, sandboxRoute.Upstream.ClusterKey,
		"sandbox cluster name should be <env>_<hash> derived from sha256(apiID)")

	_, mainExists := rdc.UpstreamClusters[expectedMain]
	require.True(t, mainExists, "main cluster %q must be registered in UpstreamClusters", expectedMain)
	_, sandboxExists := rdc.UpstreamClusters[expectedSandbox]
	require.True(t, sandboxExists, "sandbox cluster %q must be registered in UpstreamClusters", expectedSandbox)
}

// TestRestAPITransformer_APILevelDefaultClusterMatchesRealCluster verifies that
// route.Upstream.DefaultCluster matches a cluster registered in
// rdc.UpstreamClusters whenever UseClusterHeader is enabled. The policy engine
// writes DefaultCluster into the x-target-upstream header and Envoy looks up
// the cluster by that value; if the name does not match a registered cluster,
// Envoy returns 503 NoRoute.
func TestRestAPITransformer_APILevelDefaultClusterMatchesRealCluster(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users"},
	})
	// Add an upstreamDefinition so UseClusterHeader becomes true and
	// DefaultCluster is actually populated.
	spec := cfg.Configuration.(api.RestAPI)
	spec.Spec.UpstreamDefinitions = &[]api.UpstreamDefinition{
		{
			Name: "stub-def",
			Upstreams: []struct {
				Url    string `json:"url" yaml:"url"`
				Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
			}{
				{Url: "http://stub-def-svc:8080"},
			},
		},
	}
	cfg.Configuration = spec

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	require.True(t, mainRoute.Upstream.UseClusterHeader,
		"upstreamDefinitions present, UseClusterHeader should be true so DefaultCluster is meaningful")
	require.NotEmpty(t, mainRoute.Upstream.DefaultCluster,
		"DefaultCluster must be populated when UseClusterHeader is true")

	_, exists := rdc.UpstreamClusters[mainRoute.Upstream.DefaultCluster]
	assert.True(t, exists,
		"DefaultCluster %q must reference a real registered cluster in UpstreamClusters "+
			"(prevents 503 NoRoute when policy engine writes x-target-upstream)",
		mainRoute.Upstream.DefaultCluster)
	assert.Equal(t, mainRoute.Upstream.ClusterKey, mainRoute.Upstream.DefaultCluster,
		"DefaultCluster and ClusterKey must be the same string")
}

// TestRestAPITransformer_APILevelURLStableAcrossURLEdit asserts that editing the
// API-level main upstream URL does NOT change the cluster name. This is the
// URL-stable contract: the route keeps pointing at the same named cluster and
// name-keyed stats stay continuous across URL edits.
func TestRestAPITransformer_APILevelURLStableAcrossURLEdit(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	cfgA := makeRestAPIWithOps([]api.Operation{{Method: "GET", Path: "/users"}})
	rdcA, err := transformer.Transform(cfgA)
	require.NoError(t, err)

	cfgB := makeRestAPIWithOps([]api.Operation{{Method: "GET", Path: "/users"}})
	specB := cfgB.Configuration.(api.RestAPI)
	specB.Spec.Upstream.Main.Url = ptrStr("http://api-main-v2:9090")
	cfgB.Configuration = specB
	rdcB, err := transformer.Transform(cfgB)
	require.NoError(t, err)

	nameA := rdcA.Routes["GET|/test/users|main.local"].Upstream.ClusterKey
	nameB := rdcB.Routes["GET|/test/users|main.local"].Upstream.ClusterKey
	assert.Equal(t, nameA, nameB,
		"API-level main cluster name must not depend on URL "+
			"(URL-stable contract: the name must survive URL edits)")
}

// TestRestAPITransformer_APILevelMainOnlyHasNoSandboxCluster verifies that an
// API with no sandbox upstream registers no sandbox_<hash> cluster and creates
// no sandbox route. The optional env must not leave a route pointing at a
// cluster absent from UpstreamClusters (which would surface as 503 NoRoute).
func TestRestAPITransformer_APILevelMainOnlyHasNoSandboxCluster(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users"},
	})
	spec := cfg.Configuration.(api.RestAPI)
	spec.Spec.Upstream.Sandbox = nil // main-only API
	cfg.Configuration = spec

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	// Expected name is hard-coded (sha256("test-api")[:12]), not computed via
	// clusterkey.APILevel, so a change to the hashing function is caught here.
	expectedMain := "main_2a28373e2cacc6ea903d8c7e"
	expectedSandbox := "sandbox_2a28373e2cacc6ea903d8c7e"

	_, mainExists := rdc.UpstreamClusters[expectedMain]
	require.True(t, mainExists, "main cluster %q must still be registered", expectedMain)

	_, sandboxExists := rdc.UpstreamClusters[expectedSandbox]
	assert.False(t, sandboxExists,
		"sandbox cluster %q must not be registered when no sandbox upstream is configured", expectedSandbox)

	_, sandboxRouteExists := rdc.Routes["GET|/test/users|sandbox.local"]
	assert.False(t, sandboxRouteExists,
		"no sandbox route should exist for a main-only API")
}

// TestRestAPITransformer_ClusterNameUsesSharedHelper locks the cross-builder
// naming contract: the transform path names the cluster exactly
// clusterkey.APILevelName(env, cfg.UUID), the same helper and argument the xDS
// translator uses (pinned on that side in pkg/xds tests), so the two builders
// cannot drift to different names for the same API.
func TestRestAPITransformer_ClusterNameUsesSharedHelper(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users"},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	assert.Equal(t, clusterkey.APILevelName("main", cfg.UUID),
		rdc.Routes["GET|/test/users|main.local"].Upstream.ClusterKey)
	assert.Equal(t, clusterkey.APILevelName("sandbox", cfg.UUID),
		rdc.Routes["GET|/test/users|sandbox.local"].Upstream.ClusterKey)
}

