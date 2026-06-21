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

// Package clusterkey produces deterministic Envoy cluster-key fragments for the
// gateway-controller, shared by both xDS builders so they name clusters identically.
package clusterkey

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/wso2/api-platform/gateway/gateway-controller/pkg/constants"
)

// APILevel returns the 24-hex cluster-key fragment for an API-level upstream,
// the first 12 bytes of SHA-256(apiID). The URL is excluded so the name is stable
// across URL edits; main and sandbox share it, set apart by the caller's env prefix.
func APILevel(apiID string) string {
	sum := sha256.Sum256([]byte(apiID))
	return hex.EncodeToString(sum[:12])
}

// APILevelName joins the env prefix ("main"/"sandbox") to the APILevel fragment
// to form the full Envoy cluster name.
func APILevelName(env, apiID string) string {
	return env + "_" + APILevel(apiID)
}

// DefinitionName returns the full Envoy cluster name for an upstreamDefinition,
// formatted as "upstream_<kind>_<apiID>_<sanitized name>". Dots and colons in the
// definition name are replaced so the result is a valid Envoy cluster name. Both
// xDS builders use this so they name definition clusters identically.
func DefinitionName(kind, apiID, defName string) string {
	return constants.UpstreamDefinitionClusterPrefix + kind + "_" + apiID + "_" + sanitizeDefName(defName)
}

// sanitizeDefName replaces dots and colons, which are not allowed in Envoy cluster names.
func sanitizeDefName(name string) string {
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}
