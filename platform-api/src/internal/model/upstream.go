/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package model

// UpstreamConfig represents the upstream configuration with main and sandbox endpoints
type UpstreamConfig struct {
	Main    *UpstreamEndpoint `json:"main,omitempty" db:"-"`
	Sandbox *UpstreamEndpoint `json:"sandbox,omitempty" db:"-"`
}

// UpstreamEndpoint represents an upstream endpoint configuration
type UpstreamEndpoint struct {
	URL  string        `json:"url,omitempty" db:"-"`
	Ref  string        `json:"ref,omitempty" db:"-"`
	Auth *UpstreamAuth `json:"auth,omitempty" db:"-"`
}

type UpstreamAuth struct {
	Type   string `json:"type" db:"-"`
	Header string `json:"header,omitempty" db:"-"`
	Value  string `json:"value,omitempty" db:"-"`
}

// UpstreamDefinition is a reusable named upstream pool entry (the API-level
// upstreamDefinitions array). An API-level or operation-level upstream `ref`
// resolves against these by name. Mirrors the gateway upstreamDefinitions entry.
type UpstreamDefinition struct {
	Name      string            `json:"name" db:"-"`
	BasePath  string            `json:"basePath,omitempty" db:"-"`
	Timeout   *UpstreamTimeout  `json:"timeout,omitempty" db:"-"`
	Upstreams []UpstreamBackend `json:"upstreams" db:"-"`
}

// UpstreamBackend is a single backend target within an UpstreamDefinition.
type UpstreamBackend struct {
	URL    string `json:"url" db:"-"`
	Weight *int   `json:"weight,omitempty" db:"-"`
}

// UpstreamTimeout holds timeout configuration for an upstream definition.
type UpstreamTimeout struct {
	Connect string `json:"connect,omitempty" db:"-"`
}

// OperationUpstream is a per-operation upstream override (ref-only). A missing
// sub-field falls back to the API-level upstream for that vhost.
type OperationUpstream struct {
	Main    *OperationUpstreamTarget `json:"main,omitempty" db:"-"`
	Sandbox *OperationUpstreamTarget `json:"sandbox,omitempty" db:"-"`
}

// OperationUpstreamTarget is a ref-only per-operation upstream pointer. URLs are
// not permitted at the operation level; the ref must name an upstreamDefinition.
type OperationUpstreamTarget struct {
	Ref string `json:"ref" db:"-"`
}
