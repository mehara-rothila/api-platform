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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api "github.com/wso2/api-platform/gateway/gateway-controller/pkg/api/management"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/config"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/models"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/utils"
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

// TestSanitizeUpstreamDefinitionName verifies that dots and colons are replaced
// for Envoy cluster name compatibility.
func TestSanitizeUpstreamDefinitionName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-upstream", "my-upstream"},
		{"my.upstream", "my_upstream"},
		{"my:upstream", "my_upstream"},
		{"host.example.com:8080", "host_example_com_8080"},
		{"", ""},
		{"a.b.c:d", "a_b_c_d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeUpstreamDefinitionName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
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
		got, err := resolveUpstreamURL("main", up, nil)
		require.NoError(t, err)
		assert.Equal(t, u, got)
	})

	t.Run("ref to existing definition", func(t *testing.T) {
		up := &api.Upstream{Ref: &refName}
		got, err := resolveUpstreamURL("main", up, defs)
		require.NoError(t, err)
		assert.Equal(t, defURL, got)
	})

	t.Run("ref but no definitions provided", func(t *testing.T) {
		up := &api.Upstream{Ref: &refName}
		_, err := resolveUpstreamURL("main", up, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), refName)
	})

	t.Run("ref to unknown definition", func(t *testing.T) {
		unknownRef := "unknown-def"
		up := &api.Upstream{Ref: &unknownRef}
		_, err := resolveUpstreamURL("main", up, defs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("neither URL nor ref", func(t *testing.T) {
		up := &api.Upstream{}
		_, err := resolveUpstreamURL("main", up, defs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no URL or ref")
	})

	t.Run("whitespace-only URL treated as missing", func(t *testing.T) {
		blank := "   "
		up := &api.Upstream{Url: &blank}
		_, err := resolveUpstreamURL("main", up, nil)
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
		_, err := resolveUpstreamURL("main", up, emptyDefs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no URLs")
	})
}

// makeRestAPIWithOps builds a RestAPI StoredConfig with caller-supplied operations and
// both API-level main and sandbox upstreams configured (so per-op tests can observe
// sandbox-vhost route behavior).
func makeRestAPIWithOps(ops []api.Operation) *models.StoredConfig {
	apiData := api.APIConfigData{
		DisplayName: "Test API",
		Context:     "/test",
		Version:     "1.0.0",
		Operations:  ops,
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
// causes the main vhost route to use the per-op cluster while the sandbox vhost route
// falls back to the API-level sandbox cluster.
func TestRestAPITransformer_PerOpMainOverridesMainVhost(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://user-svc:8080")},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)
	require.NotNil(t, rdc)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	assert.True(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "op_"),
		"main vhost should use per-op cluster, got %q", mainRoute.Upstream.ClusterKey)
	assert.False(t, mainRoute.Upstream.UseClusterHeader)

	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, sandboxRoute)
	assert.False(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "op_"),
		"sandbox vhost should fall back to API sandbox, got %q", sandboxRoute.Upstream.ClusterKey)
}

// TestRestAPITransformer_PerOpSandboxOverridesSandboxVhost asserts that a sandbox-only override
// causes the main vhost to fall back to the API main while the sandbox vhost uses the per-op cluster.
func TestRestAPITransformer_PerOpSandboxOverridesSandboxVhost(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Sandbox: &api.Upstream{Url: ptrStr("http://user-svc-test:8080")},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	assert.False(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "op_"),
		"main vhost should fall back to API main, got %q", mainRoute.Upstream.ClusterKey)

	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, sandboxRoute)
	assert.True(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "op_"),
		"sandbox vhost should use per-op cluster, got %q", sandboxRoute.Upstream.ClusterKey)
}

// TestRestAPITransformer_PerOpBothOverrideBothVhosts asserts that both vhosts get distinct
// per-op clusters when main and sandbox are overridden.
func TestRestAPITransformer_PerOpBothOverrideBothVhosts(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main:    &api.Upstream{Url: ptrStr("http://user-svc:8080")},
				Sandbox: &api.Upstream{Url: ptrStr("http://user-svc-test:8080")},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, mainRoute)
	require.NotNil(t, sandboxRoute)

	assert.True(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "op_"))
	assert.True(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "op_"))
	assert.NotEqual(t, mainRoute.Upstream.ClusterKey, sandboxRoute.Upstream.ClusterKey,
		"main and sandbox per-op vhosts must produce distinct cluster keys (env differs)")
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
	assert.False(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "op_"))
	assert.False(t, strings.HasPrefix(sandboxRoute.Upstream.ClusterKey, "op_"))
}

// TestRestAPITransformer_PerOpDistinctOpsSameURL asserts that two distinct operations
// pointing at the same backend URL produce separate clusters because the hash includes
// method and path.
func TestRestAPITransformer_PerOpDistinctOpsSameURL(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://shared-svc:8080")},
			},
		},
		{
			Method: "POST", Path: "/orders",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://shared-svc:8080")},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	opCount := 0
	for k := range rdc.UpstreamClusters {
		if strings.HasPrefix(k, "op_") {
			opCount++
		}
	}
	assert.Equal(t, 2, opCount, "two distinct ops with same URL should produce two clusters under per-op naming")
}

// TestRestAPITransformer_PerOpSameOpDifferentURLsDedup asserts that two operations with
// identical (method, path, env) but different backend URLs collapse into a single per-op
// cluster, because URL is intentionally excluded from the hash (EDS-stable design).
func TestRestAPITransformer_PerOpSameOpDifferentURLsDedup(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://user-svc:8080")},
			},
		},
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://user-svc:9090")},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	opCount := 0
	for k := range rdc.UpstreamClusters {
		if strings.HasPrefix(k, "op_") {
			opCount++
		}
	}
	assert.Equal(t, 1, opCount,
		"identical (method, path, env) must collapse to one cluster regardless of URL difference (URL excluded from hash)")
}

// TestRestAPITransformer_PerOpClusterIsolatedAcrossAPIs asserts that two APIs with the
// same operation pointing at the same backend URL produce different per-op cluster keys
// because apiID is part of the hash input.
func TestRestAPITransformer_PerOpClusterIsolatedAcrossAPIs(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	cfgA := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://shared-svc:8080")},
			},
		},
	})
	cfgA.UUID = "api-aaa"

	cfgB := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://shared-svc:8080")},
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
		if strings.HasPrefix(k, "op_") {
			keyA = k
		}
	}
	for k := range rdcB.UpstreamClusters {
		if strings.HasPrefix(k, "op_") {
			keyB = k
		}
	}

	require.NotEmpty(t, keyA)
	require.NotEmpty(t, keyB)
	assert.NotEqual(t, keyA, keyB, "same URL across different APIs must produce different per-op cluster keys")
}

// TestRestAPITransformer_PerOpSandboxWithoutAPILevelSandbox - guard regression.
// API-level Sandbox is nil, but one op declares a per-op sandbox upstream. The
// sandbox vhost must be created only for that op; ops without per-op sandbox
// must NOT get a sandbox route (otherwise they'd silently route to the main
// cluster on the sandbox vhost).
func TestRestAPITransformer_PerOpSandboxWithoutAPILevelSandbox(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	apiData := api.APIConfigData{
		DisplayName: "Test API",
		Context:     "/test",
		Version:     "1.0.0",
		Operations: []api.Operation{
			{
				Method: "GET", Path: "/users",
				Upstream: &api.OperationUpstream{
					Sandbox: &api.Upstream{Url: ptrStr("http://user-svc-test:8080")},
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
	assert.False(t, strings.HasPrefix(usersMain.Upstream.ClusterKey, "op_"),
		"main vhost should fall back to API main cluster, got %q", usersMain.Upstream.ClusterKey)

	usersSandbox := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, usersSandbox, "op with per-op sandbox must have a sandbox route")
	assert.True(t, strings.HasPrefix(usersSandbox.Upstream.ClusterKey, "op_"),
		"sandbox vhost should use per-op cluster, got %q", usersSandbox.Upstream.ClusterKey)

	ordersMain := rdc.Routes["GET|/test/orders|main.local"]
	require.NotNil(t, ordersMain, "op without per-op upstream must have a main route")
	assert.False(t, strings.HasPrefix(ordersMain.Upstream.ClusterKey, "op_"))

	_, ordersHasSandbox := rdc.Routes["GET|/test/orders|sandbox.local"]
	assert.False(t, ordersHasSandbox,
		"op without per-op sandbox must NOT get a sandbox route when API-level sandbox is nil")
}

// TestRestAPITransformer_PerOpMainSandboxSameURL - when Main and Sandbox
// sub-fields of the same operation point at the same URL, they still get separate
// clusters because the env label ("main" vs "sandbox") differs.
func TestRestAPITransformer_PerOpMainSandboxSameURL(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main:    &api.Upstream{Url: ptrStr("http://same-svc:8080")},
				Sandbox: &api.Upstream{Url: ptrStr("http://same-svc:8080")},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	opCount := 0
	for k := range rdc.UpstreamClusters {
		if strings.HasPrefix(k, "op_") {
			opCount++
		}
	}
	assert.Equal(t, 2, opCount, "Main and Sandbox are separate envs → separate clusters even with same URL")
}

// TestRestAPITransformer_PerOpMainHostRewriteManualDisablesAuto asserts that setting
// HostRewrite to Manual on a per-op main upstream disables AutoHostRewrite on the route.
func TestRestAPITransformer_PerOpMainHostRewriteManualDisablesAuto(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	manual := api.Manual
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://user-svc:8080"), HostRewrite: &manual},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	assert.False(t, mainRoute.AutoHostRewrite,
		"per-op main with HostRewrite=manual must disable AutoHostRewrite")
}

// TestRestAPITransformer_PerOpMainHostRewriteAutoLeavesAutoEnabled asserts that leaving
// HostRewrite as Auto (or nil) on a per-op main upstream keeps AutoHostRewrite enabled.
func TestRestAPITransformer_PerOpMainHostRewriteAutoLeavesAutoEnabled(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	auto := api.Auto
	cfg := makeRestAPIWithOps([]api.Operation{
		{
			Method: "GET", Path: "/users",
			Upstream: &api.OperationUpstream{
				Main: &api.Upstream{Url: ptrStr("http://user-svc:8080"), HostRewrite: &auto},
			},
		},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute)
	assert.True(t, mainRoute.AutoHostRewrite,
		"per-op main with HostRewrite=auto must keep AutoHostRewrite enabled")
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

// TestRestAPITransformer_PerOpRefPropagatesConnectTimeout - when a per-op upstream
// uses a ref to an upstreamDefinition that has a connect timeout, the timeout must
// flow into the resulting per-op UpstreamCluster.ConnectTimeout field. Without this,
// per-op clusters built from refs would silently drop the timeout configured on the
// referenced definition.
func TestRestAPITransformer_PerOpRefPropagatesConnectTimeout(t *testing.T) {
	connectTimeout := "7s"
	upstreamDefinitions := []api.UpstreamDefinition{
		{
			Name: "user-svc-cluster",
			Timeout: &api.UpstreamTimeout{
				Connect: &connectTimeout,
			},
			Upstreams: []struct {
				Url    string `json:"url" yaml:"url"`
				Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
			}{
				{Url: "http://user-svc:8080"},
			},
		},
	}

	refName := "user-svc-cluster"
	apiData := api.APIConfigData{
		DisplayName:         "Test API",
		Context:             "/test",
		Version:             "1.0.0",
		UpstreamDefinitions: &upstreamDefinitions,
		Operations: []api.Operation{
			{
				Method: "GET", Path: "/users",
				Upstream: &api.OperationUpstream{
					Main: &api.Upstream{Ref: &refName},
				},
			},
		},
		Upstream: struct {
			Main    api.Upstream  `json:"main" yaml:"main"`
			Sandbox *api.Upstream `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
		}{
			Main: api.Upstream{Url: ptrStr("http://api-main:8080")},
		},
	}
	cfg := &models.StoredConfig{
		UUID:          "test-api",
		Kind:          string(api.RestAPIKindRestApi),
		Configuration: api.RestAPI{Kind: api.RestAPIKindRestApi, Metadata: api.Metadata{Name: "test-api"}, Spec: apiData},
	}

	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute, "main route must exist for the per-op operation")
	require.True(t, strings.HasPrefix(mainRoute.Upstream.ClusterKey, "op_"),
		"main vhost should use a per-op cluster, got %q", mainRoute.Upstream.ClusterKey)

	cluster, ok := rdc.UpstreamClusters[mainRoute.Upstream.ClusterKey]
	require.True(t, ok, "per-op cluster %q must exist in UpstreamClusters map", mainRoute.Upstream.ClusterKey)
	require.NotNil(t, cluster.ConnectTimeout,
		"per-op cluster built from ref must inherit ConnectTimeout from the upstreamDefinition")
	assert.Equal(t, 7*time.Second, *cluster.ConnectTimeout,
		"ConnectTimeout should be 7s (parsed from the definition's '7s')")
}

// TestRestAPITransformer_APILevelClusterNameShape asserts the EDS-stable cluster
// naming contract for API-level main and sandbox upstreams:
//   - cluster names are "<env>_<16-hex>" derived from sha256(apiID|env)
//   - ClusterKey and EnvoyClusterName are the SAME string (so the policy engine's
//     default_upstream_cluster metadata resolves to a real Envoy cluster)
func TestRestAPITransformer_APILevelClusterNameShape(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})
	cfg := makeRestAPIWithOps([]api.Operation{
		{Method: "GET", Path: "/users"},
	})

	rdc, err := transformer.Transform(cfg)
	require.NoError(t, err)

	expectedMain := "main_" + utils.APILevelClusterKey(cfg.UUID, "main")
	expectedSandbox := "sandbox_" + utils.APILevelClusterKey(cfg.UUID, "sandbox")

	mainRoute := rdc.Routes["GET|/test/users|main.local"]
	require.NotNil(t, mainRoute, "main route must exist")
	assert.Equal(t, expectedMain, mainRoute.Upstream.ClusterKey,
		"main cluster name should be <env>_<hash> derived from apiID|env")

	sandboxRoute := rdc.Routes["GET|/test/users|sandbox.local"]
	require.NotNil(t, sandboxRoute, "sandbox route must exist")
	assert.Equal(t, expectedSandbox, sandboxRoute.Upstream.ClusterKey,
		"sandbox cluster name should be <env>_<hash> derived from apiID|env")

	_, mainExists := rdc.UpstreamClusters[expectedMain]
	require.True(t, mainExists, "main cluster %q must be registered in UpstreamClusters", expectedMain)
	_, sandboxExists := rdc.UpstreamClusters[expectedSandbox]
	require.True(t, sandboxExists, "sandbox cluster %q must be registered in UpstreamClusters", expectedSandbox)
}

// TestRestAPITransformer_APILevelDefaultClusterMatchesRealCluster is a regression
// guard for a latent 503 NoRoute bug. The policy engine reads
// route.Upstream.DefaultCluster and writes it into the x-target-upstream header
// when the default upstream path is taken. Envoy then looks up the cluster by
// that header value. If DefaultCluster does not match a registered cluster
// name, Envoy returns 503 NR.
//
// Previously DefaultCluster was sanitizeEnvoyClusterName(host, scheme) while the
// real cluster was registered as "upstream_main_<host>_<port>" - mismatch.
// The fix unifies them. This test pins that contract.
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
		"DefaultCluster and ClusterKey must be the same string after the unification fix")
}

// TestRestAPITransformer_APILevelEDSStableAcrossURLEdit asserts that editing the
// API-level main upstream URL does NOT change the cluster name. This is the
// EDS-stable contract: URL edits propagate as endpoint updates, not cluster
// recreates.
func TestRestAPITransformer_APILevelEDSStableAcrossURLEdit(t *testing.T) {
	transformer := NewRestAPITransformer(testRouterCfg(), &config.Config{}, map[string]models.PolicyDefinition{})

	cfgA := makeRestAPIWithOps([]api.Operation{{Method: "GET", Path: "/users"}})
	// Mutate just the main URL on a copy of the spec.
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
			"(EDS-stable contract: URL edits propagate via endpoint updates)")
}

