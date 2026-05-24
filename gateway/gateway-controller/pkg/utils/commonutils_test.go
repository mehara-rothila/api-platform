/*
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/constants"
)

func TestGetParamsOfPolicy(t *testing.T) {
	tests := []struct {
		name      string
		policyDef string
		params    []string
		expected  map[string]any
		wantErr   bool
	}{
		{
			name: "Valid YAML with parameters",
			policyDef: `
key: %s
value: %s
`,
			params: []string{"testKey", "testValue"},
			expected: map[string]any{
				"key":   "testKey",
				"value": "testValue",
			},
			wantErr: false,
		},
		{
			name: "Valid YAML without parameters",
			policyDef: `
key: staticValue
`,
			params: []string{},
			expected: map[string]any{
				"key": "staticValue",
			},
			wantErr: false,
		},
		{
			name: "Invalid YAML",
			policyDef: `
key: : value
`,
			params:   []string{},
			expected: map[string]any{},
			wantErr:  true,
		},
		{
			name: "Valid YAML with nested structure",
			policyDef: `
parent:
  child: %s
`,
			params: []string{"childValue"},
			expected: map[string]any{
				"parent": map[string]any{
					"child": "childValue",
				},
			},
			wantErr: false,
		},
		{
			name:      "Valid SET_HEADERS_POLICY_PARAMS",
			policyDef: constants.SET_HEADERS_POLICY_PARAMS,
			params:    []string{"Authorization", "Bearer token"},
			expected: map[string]any{
				"request": map[string]any{
					"headers": []any{
						map[string]any{
							"name":  "Authorization",
							"value": "Bearer token",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetParamsOfPolicy(tt.policyDef, tt.params...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

// TestPerOpClusterKey verifies the cluster-key hash function pins the contract
// the per-op upstream feature depends on:
//   - deterministic for identical inputs
//   - distinct on any input field (apiID, method, path, env)
//   - case-insensitive on method (uppercased before hashing)
//
// The URL-independence contract (URL changes must not alter the cluster key) is
// asserted end-to-end at the xds layer where the cluster name is actually emitted;
// see pkg/xds/translator_test.go TestResolvePerOpUpstream_DedupSameURL.
func TestPerOpClusterKey(t *testing.T) {
	t.Run("deterministic for identical inputs", func(t *testing.T) {
		a := PerOpClusterKey("api-1", "GET", "/users", "main")
		b := PerOpClusterKey("api-1", "GET", "/users", "main")
		assert.Equal(t, a, b, "same inputs must produce same hash")
		assert.Len(t, a, 16, "hash must be 16 hex chars (8 bytes of SHA-256)")
	})

	t.Run("method casing is normalized to uppercase", func(t *testing.T) {
		upper := PerOpClusterKey("api-1", "GET", "/users", "main")
		lower := PerOpClusterKey("api-1", "get", "/users", "main")
		mixed := PerOpClusterKey("api-1", "Get", "/users", "main")
		assert.Equal(t, upper, lower, "lowercase method must hash to same key as uppercase")
		assert.Equal(t, upper, mixed, "mixed-case method must hash to same key as uppercase")
	})

	t.Run("different apiID produces different hash", func(t *testing.T) {
		a := PerOpClusterKey("api-1", "GET", "/users", "main")
		b := PerOpClusterKey("api-2", "GET", "/users", "main")
		assert.NotEqual(t, a, b)
	})

	t.Run("different method produces different hash", func(t *testing.T) {
		a := PerOpClusterKey("api-1", "GET", "/users", "main")
		b := PerOpClusterKey("api-1", "POST", "/users", "main")
		assert.NotEqual(t, a, b)
	})

	t.Run("different path produces different hash", func(t *testing.T) {
		a := PerOpClusterKey("api-1", "GET", "/users", "main")
		b := PerOpClusterKey("api-1", "GET", "/orders", "main")
		assert.NotEqual(t, a, b)
	})

	t.Run("different env produces different hash", func(t *testing.T) {
		a := PerOpClusterKey("api-1", "GET", "/users", "main")
		b := PerOpClusterKey("api-1", "GET", "/users", "sandbox")
		assert.NotEqual(t, a, b)
	})

}

// TestAPILevelClusterKey validates the contract for API-level cluster naming:
//   - deterministic for identical inputs
//   - distinct on any input field (apiID, env)
//   - same hash family as PerOpClusterKey (16 hex chars from SHA-256[:8])
//
// The URL-independence contract is asserted end-to-end at the xds layer where
// the cluster name is actually emitted.
func TestAPILevelClusterKey(t *testing.T) {
	t.Run("deterministic for identical inputs", func(t *testing.T) {
		a := APILevelClusterKey("api-1", "main")
		b := APILevelClusterKey("api-1", "main")
		assert.Equal(t, a, b, "same inputs must produce same hash")
		assert.Len(t, a, 16, "hash must be 16 hex chars (8 bytes of SHA-256)")
	})

	t.Run("different apiID produces different hash", func(t *testing.T) {
		a := APILevelClusterKey("api-1", "main")
		b := APILevelClusterKey("api-2", "main")
		assert.NotEqual(t, a, b)
	})

	t.Run("different env produces different hash", func(t *testing.T) {
		a := APILevelClusterKey("api-1", "main")
		b := APILevelClusterKey("api-1", "sandbox")
		assert.NotEqual(t, a, b)
	})

	t.Run("does not collide with PerOpClusterKey for same apiID and env", func(t *testing.T) {
		// Distinct algorithms (different hash inputs) so the hex tails should
		// almost certainly differ. This is a sanity check; callers also add
		// different prefixes ("main_" vs "op_") so full cluster names cannot
		// collide regardless.
		apiLevel := APILevelClusterKey("api-1", "main")
		perOp := PerOpClusterKey("api-1", "GET", "/users", "main")
		assert.NotEqual(t, apiLevel, perOp,
			"API-level and per-op hashes for the same apiID/env must differ "+
				"because per-op also folds method and path into the input")
	})
}
