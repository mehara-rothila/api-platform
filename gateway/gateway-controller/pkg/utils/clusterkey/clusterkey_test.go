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

package clusterkey

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// hexShape16 matches exactly 16 lowercase hex characters - the cluster-key
// fragment shape produced by APILevel.
var hexShape16 = regexp.MustCompile("^[a-f0-9]{16}$")

// TestAPILevel validates the contract for API-level cluster naming:
//   - deterministic for identical inputs
//   - distinct on any input field (apiID, env)
//   - 16 hex chars from SHA-256[:8]
func TestAPILevel(t *testing.T) {
	t.Run("deterministic for identical inputs", func(t *testing.T) {
		a := APILevel("api-1", "main")
		b := APILevel("api-1", "main")
		assert.Equal(t, a, b, "same inputs must produce same hash")
		assert.Regexp(t, hexShape16, a, "hash must be exactly 16 lowercase hex characters")
	})

	t.Run("different apiID produces different hash", func(t *testing.T) {
		a := APILevel("api-1", "main")
		b := APILevel("api-2", "main")
		assert.NotEqual(t, a, b)
	})

	t.Run("different env produces different hash", func(t *testing.T) {
		a := APILevel("api-1", "main")
		b := APILevel("api-1", "sandbox")
		assert.NotEqual(t, a, b)
	})
}
