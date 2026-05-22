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
//   - the URL is NOT a hash input (so URL edits become EDS updates, not CDS rebuilds)
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

	t.Run("URL is NOT a hash input (EDS-stable cluster naming)", func(t *testing.T) {
		// The whole point of the hash design: the URL is deliberately excluded
		// so an upstream URL change becomes an EDS endpoint update (no connection
		// drain), not a CDS cluster recreate. PerOpClusterKey takes 4 args; URL
		// is not one of them. This test pins that contract at the API surface —
		// if someone ever adds a URL parameter, the function signature changes
		// and this test must change with it, forcing a deliberate decision.
		a := PerOpClusterKey("api-1", "GET", "/users", "main")
		b := PerOpClusterKey("api-1", "GET", "/users", "main")
		assert.Equal(t, a, b,
			"same (apiID, method, path, env) must produce same hash regardless of any external URL")
	})
}
