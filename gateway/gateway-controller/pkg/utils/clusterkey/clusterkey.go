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

// Package clusterkey produces deterministic, hex-encoded cluster-key fragments
// used by the gateway-controller to name Envoy clusters. It is a leaf package
// (stdlib imports only) so both pkg/utils and pkg/xds can depend on it without
// forming an import cycle.
package clusterkey

import (
	"crypto/sha256"
	"encoding/hex"
)

// APILevel returns a deterministic, hex-encoded cluster-key fragment for an
// API-level upstream cluster (main or sandbox). The key is derived from
// SHA-256 of apiID|env. Method and path are excluded because API-level
// upstream applies to the whole API; the URL is excluded so URL edits update
// endpoints in-place rather than destroying and recreating the cluster.
func APILevel(apiID, env string) string {
	hashInput := apiID + "|" + env
	sum := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(sum[:8])
}
