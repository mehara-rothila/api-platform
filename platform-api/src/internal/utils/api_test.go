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

package utils

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"platform-api/src/api"
	"platform-api/src/internal/constants"
	"platform-api/src/internal/dto"
	"platform-api/src/internal/model"
)

// TestPolicyAPIToModel tests conversion from generated API Policy to Model Policy
func TestPolicyAPIToModel(t *testing.T) {
	util := &APIUtil{}

	executionCondition := "request.path == '/api/v1/users'"
	params := map[string]interface{}{
		"rateLimit": 100,
		"timeUnit":  "minute",
	}

	tests := []struct {
		name     string
		input    *api.Policy
		expected *model.Policy
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name: "all fields set",
			input: &api.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "rate-limiting",
				Params:             &params,
				Version:            "v1",
			},
			expected: &model.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "rate-limiting",
				Params:             &params,
				Version:            "v1",
			},
		},
		{
			name: "nil ExecutionCondition",
			input: &api.Policy{
				ExecutionCondition: nil,
				Name:               "logging",
				Params:             &params,
				Version:            "v2",
			},
			expected: &model.Policy{
				ExecutionCondition: nil,
				Name:               "logging",
				Params:             &params,
				Version:            "v2",
			},
		},
		{
			name: "nil Params",
			input: &api.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "authentication",
				Params:             nil,
				Version:            "v1",
			},
			expected: &model.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "authentication",
				Params:             nil,
				Version:            "v1",
			},
		},
		{
			name: "empty Params",
			input: &api.Policy{
				ExecutionCondition: nil,
				Name:               "cors",
				Params:             &map[string]interface{}{},
				Version:            "v1",
			},
			expected: &model.Policy{
				ExecutionCondition: nil,
				Name:               "cors",
				Params:             &map[string]interface{}{},
				Version:            "v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.PolicyAPIToModel(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("PolicyAPIToModel() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestPolicyModelToAPI tests conversion from Model Policy to generated API Policy
func TestPolicyModelToAPI(t *testing.T) {
	util := &APIUtil{}

	executionCondition := "response.status == 200"
	params := map[string]interface{}{
		"cacheTTL": 3600,
		"enabled":  true,
	}

	tests := []struct {
		name     string
		input    *model.Policy
		expected *api.Policy
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name: "all fields set",
			input: &model.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "caching",
				Params:             &params,
				Version:            "v1",
			},
			expected: &api.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "caching",
				Params:             &params,
				Version:            "v1",
			},
		},
		{
			name: "nil ExecutionCondition",
			input: &model.Policy{
				ExecutionCondition: nil,
				Name:               "throttling",
				Params:             &params,
				Version:            "v3",
			},
			expected: &api.Policy{
				ExecutionCondition: nil,
				Name:               "throttling",
				Params:             &params,
				Version:            "v3",
			},
		},
		{
			name: "nil Params",
			input: &model.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "header-modifier",
				Params:             nil,
				Version:            "v2",
			},
			expected: &api.Policy{
				ExecutionCondition: &executionCondition,
				Name:               "header-modifier",
				Params:             nil,
				Version:            "v2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input == nil {
				if tt.expected != nil {
					t.Errorf("PolicyModelToAPI() = nil, want %v", tt.expected)
				}
				return
			}
			result := util.PolicyModelToAPI(*tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("PolicyModelToAPI() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestPoliciesAPIToModel tests conversion from generated API Policy slice to Model Policy slice
func TestPoliciesAPIToModel(t *testing.T) {
	util := &APIUtil{}

	condition1 := "request.method == 'POST'"
	params1 := map[string]interface{}{"maxSize": 1024}
	condition2 := "response.status >= 400"
	params2 := map[string]interface{}{"logLevel": "error"}

	tests := []struct {
		name     string
		input    *[]api.Policy
		expected []model.Policy
	}{
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    &[]api.Policy{},
			expected: []model.Policy{},
		},
		{
			name: "single policy",
			input: &[]api.Policy{
				{
					ExecutionCondition: &condition1,
					Name:               "validation",
					Params:             &params1,
					Version:            "v1",
				},
			},
			expected: []model.Policy{
				{
					ExecutionCondition: &condition1,
					Name:               "validation",
					Params:             &params1,
					Version:            "v1",
				},
			},
		},
		{
			name: "multiple policies",
			input: &[]api.Policy{
				{
					ExecutionCondition: &condition1,
					Name:               "request-logger",
					Params:             &params1,
					Version:            "v1",
				},
				{
					ExecutionCondition: nil,
					Name:               "rate-limiter",
					Params:             nil,
					Version:            "v2",
				},
				{
					ExecutionCondition: &condition2,
					Name:               "error-logger",
					Params:             &params2,
					Version:            "v1",
				},
			},
			expected: []model.Policy{
				{
					ExecutionCondition: &condition1,
					Name:               "request-logger",
					Params:             &params1,
					Version:            "v1",
				},
				{
					ExecutionCondition: nil,
					Name:               "rate-limiter",
					Params:             nil,
					Version:            "v2",
				},
				{
					ExecutionCondition: &condition2,
					Name:               "error-logger",
					Params:             &params2,
					Version:            "v1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.PoliciesAPIToModel(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("PoliciesAPIToModel() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestPoliciesModelToAPI tests conversion from Model Policy slice to generated API Policy slice
func TestPoliciesModelToAPI(t *testing.T) {
	util := &APIUtil{}

	condition := "request.header['X-API-Key'] != ''"
	params := map[string]interface{}{"required": true}

	tests := []struct {
		name     string
		input    []model.Policy
		expected *[]api.Policy
	}{
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []model.Policy{},
			expected: &[]api.Policy{},
		},
		{
			name: "multiple policies",
			input: []model.Policy{
				{
					ExecutionCondition: &condition,
					Name:               "api-key-validation",
					Params:             &params,
					Version:            "v1",
				},
				{
					ExecutionCondition: nil,
					Name:               "jwt-validation",
					Params:             nil,
					Version:            "v2",
				},
			},
			expected: &[]api.Policy{
				{
					ExecutionCondition: &condition,
					Name:               "api-key-validation",
					Params:             &params,
					Version:            "v1",
				},
				{
					ExecutionCondition: nil,
					Name:               "jwt-validation",
					Params:             nil,
					Version:            "v2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.PoliciesModelToAPI(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("PoliciesModelToAPI() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestPolicyRoundTrip tests API -> Model -> API conversion preserves data
func TestPolicyRoundTrip(t *testing.T) {
	util := &APIUtil{}

	condition := "request.header['Content-Type'] == 'application/json'"
	params := map[string]interface{}{
		"validateSchema": true,
		"schemaVersion":  "2.0",
		"strictMode":     false,
	}

	original := &api.Policy{
		ExecutionCondition: &condition,
		Name:               "json-validator",
		Params:             &params,
		Version:            "v3",
	}

	// Convert API -> Model
	modelPolicy := util.PolicyAPIToModel(original)
	if modelPolicy == nil {
		t.Fatal("PolicyAPIToModel() returned nil")
	}

	// Convert Model -> API
	result := util.PolicyModelToAPI(*modelPolicy)
	if result == nil {
		t.Fatal("PolicyModelToAPI() returned nil")
	}

	// Verify all fields match
	if !reflect.DeepEqual(result, original) {
		t.Errorf("Round-trip conversion failed.\nOriginal: %+v\nResult: %+v", original, result)
	}

	// Verify ExecutionCondition pointer is preserved
	if result.ExecutionCondition == nil || *result.ExecutionCondition != condition {
		t.Errorf("ExecutionCondition not preserved correctly")
	}

	// Verify Params pointer is preserved
	if result.Params == nil {
		t.Error("Params pointer lost in round-trip")
	} else {
		if !reflect.DeepEqual(*result.Params, params) {
			t.Errorf("Params values changed in round-trip")
		}
	}
}

// TestOperationRequestRoundTrip tests API -> Model -> API conversion preserves Policies
func TestOperationRequestRoundTrip(t *testing.T) {
	util := &APIUtil{}

	condition1 := "request.method == 'POST'"
	params1 := map[string]interface{}{"maxBodySize": 10240}
	condition2 := "response.status == 201"

	original := &api.OperationRequest{
		Method: api.OperationRequestMethodPOST,
		Path:   "/api/v1/resources",
		Policies: &[]api.Policy{
			{
				ExecutionCondition: &condition1,
				Name:               "body-size-validator",
				Params:             &params1,
				Version:            "v1",
			},
			{
				ExecutionCondition: &condition2,
				Name:               "success-logger",
				Params:             nil,
				Version:            "v2",
			},
			{
				ExecutionCondition: nil,
				Name:               "audit-trail",
				Params:             &map[string]interface{}{"enabled": true},
				Version:            "v1",
			},
		},
	}

	// Convert API -> Model
	modelRequest := util.OperationRequestAPIToModel(original)
	if modelRequest == nil {
		t.Fatal("OperationRequestAPIToModel() returned nil")
	}

	// Convert Model -> API
	result := util.OperationRequestModelToAPI(modelRequest)
	if result == nil {
		t.Fatal("OperationRequestModelToAPI() returned nil")
	}

	// Verify all fields match
	if !reflect.DeepEqual(result, original) {
		t.Errorf("Round-trip conversion failed.\nOriginal: %+v\nResult: %+v", original, result)
	}

	// Verify Policies count
	if result.Policies == nil || original.Policies == nil {
		t.Fatal("Policies pointer should not be nil")
	}

	if len(*result.Policies) != len(*original.Policies) {
		t.Errorf("Policies count mismatch. Got %d, want %d", len(*result.Policies), len(*original.Policies))
	}

	// Verify each policy is preserved
	for i := range *result.Policies {
		if !reflect.DeepEqual((*result.Policies)[i], (*original.Policies)[i]) {
			t.Errorf("Policy[%d] not preserved.\nOriginal: %+v\nResult: %+v",
				i, (*original.Policies)[i], (*result.Policies)[i])
		}
	}
}

func TestGenerateAPIDeploymentYAMLIncludesPolicies(t *testing.T) {
	util := &APIUtil{}

	condition := "request.path == '/pets'"
	params := map[string]interface{}{"limit": 10}
	policies := []model.Policy{
		{
			ExecutionCondition: &condition,
			Name:               "rate-limit",
			Params:             &params,
			Version:            "v1",
		},
	}

	context := "/pets"
	apiModel := &model.API{
		Handle:    "api-123",
		Name:      "Pets API",
		Version:   "v1",
		ProjectID: "project-123",
		Kind:      constants.RestApi,
		Configuration: model.RestAPIConfig{
			Context:  &context,
			Policies: policies,
			Upstream: model.UpstreamConfig{
				Main: &model.UpstreamEndpoint{
					URL: "https://backend.example.com",
				},
			},
			Operations: []model.Operation{
				{
					Request: &model.OperationRequest{
						Method: "GET",
						Path:   "/pets",
					},
				},
			},
		},
	}

	yamlString, err := util.GenerateAPIDeploymentYAML(apiModel)
	if err != nil {
		t.Fatalf("GenerateAPIDeploymentYAML() error = %v", err)
	}

	var deployment dto.APIDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlString), &deployment); err != nil {
		t.Fatalf("failed to unmarshal deployment YAML: %v", err)
	}

	expectedPolicies := &[]api.Policy{
		{
			ExecutionCondition: &condition,
			Name:               "rate-limit",
			Params:             &params,
			Version:            "v1",
		},
	}

	deploymentPolicies := util.PoliciesModelToAPI(util.PoliciesDTOToModel(deployment.Spec.Policies))
	if !reflect.DeepEqual(deploymentPolicies, expectedPolicies) {
		t.Errorf("deployment policies = %v, want %v", deploymentPolicies, expectedPolicies)
	}
}

func TestAPIYAMLDataToRESTAPIPreservesPolicies(t *testing.T) {
	util := &APIUtil{}

	condition := "request.method == 'GET'"
	params := map[string]interface{}{"enabled": true}
	generatedPolicies := []api.Policy{
		{
			ExecutionCondition: &condition,
			Name:               "request-logger",
			Params:             &params,
			Version:            "v2",
		},
	}

	yamlData := &dto.APIYAMLData{
		DisplayName: "Pets API",
		Version:     "v1",
		Context:     "/pets",
		Policies:    util.PoliciesModelToDTO(util.PoliciesAPIToModel(&generatedPolicies)),
	}

	restAPI := util.APIYAMLDataToRESTAPI(yamlData)
	if restAPI == nil {
		t.Fatal("APIYAMLDataToRESTAPI() returned nil")
	}

	expectedPolicies := &generatedPolicies

	if !reflect.DeepEqual(restAPI.Policies, expectedPolicies) {
		t.Errorf("API policies = %v, want %v", restAPI.Policies, expectedPolicies)
	}
}

// TestBuildAPIDeploymentYAML verifies that BuildAPIDeploymentYAML produces the same result
// as GenerateAPIDeploymentYAML when marshalled to YAML.
func TestBuildAPIDeploymentYAML(t *testing.T) {
	util := &APIUtil{}

	ctx := "/test"
	apiModel := &model.API{
		Name:    "Test API",
		Handle:  "test-api-handle",
		Version: "v1.0",
		Kind:    constants.RestApi,
		Configuration: model.RestAPIConfig{
			Context: &ctx,
			Upstream: model.UpstreamConfig{
				Main: &model.UpstreamEndpoint{
					URL: "http://backend:8080",
				},
			},
			Operations: []model.Operation{
				{
					Request: &model.OperationRequest{
						Method: "GET",
						Path:   "/pets",
					},
				},
			},
		},
		ProjectID: "proj-123",
	}

	// Build struct
	deploymentStruct, err := util.BuildAPIDeploymentYAML(apiModel)
	if err != nil {
		t.Fatalf("BuildAPIDeploymentYAML() error = %v", err)
	}

	// Marshal the struct
	structBytes, err := yaml.Marshal(deploymentStruct)
	if err != nil {
		t.Fatalf("failed to marshal struct: %v", err)
	}

	// Generate via the wrapper
	yamlString, err := util.GenerateAPIDeploymentYAML(apiModel)
	if err != nil {
		t.Fatalf("GenerateAPIDeploymentYAML() error = %v", err)
	}

	// Compare: both should produce identical YAML
	if string(structBytes) != yamlString {
		t.Errorf("BuildAPIDeploymentYAML + Marshal differs from GenerateAPIDeploymentYAML.\nBuild:\n%s\nGenerate:\n%s", string(structBytes), yamlString)
	}

	// Verify key struct fields
	if deploymentStruct.ApiVersion != "gateway.api-platform.wso2.com/v1alpha1" {
		t.Errorf("ApiVersion = %q", deploymentStruct.ApiVersion)
	}
	if deploymentStruct.Kind != constants.RestApi {
		t.Errorf("Kind = %q", deploymentStruct.Kind)
	}
	if deploymentStruct.Metadata.Name != "test-api-handle" {
		t.Errorf("Metadata.Name = %q", deploymentStruct.Metadata.Name)
	}
	if deploymentStruct.Spec.Upstream == nil || deploymentStruct.Spec.Upstream.Main == nil {
		t.Fatal("expected upstream.main to be set")
	}
	if deploymentStruct.Spec.Upstream.Main.URL != "http://backend:8080" {
		t.Errorf("Upstream URL = %q", deploymentStruct.Spec.Upstream.Main.URL)
	}
}

// TestBuildAPIDeploymentYAML_EmitsUpstreamDefinitions verifies the reusable upstream pool is
// emitted into the deployment YAML (the gateway resolves refs against it).
func TestBuildAPIDeploymentYAML_EmitsUpstreamDefinitions(t *testing.T) {
	util := &APIUtil{}
	ctx := "/test"
	weight := 80
	apiModel := &model.API{
		Name:    "Pool API",
		Handle:  "pool-api",
		Version: "v1.0",
		Kind:    constants.RestApi,
		Configuration: model.RestAPIConfig{
			Context:  &ctx,
			Upstream: model.UpstreamConfig{Main: &model.UpstreamEndpoint{URL: "http://main:8080"}},
			UpstreamDefinitions: []model.UpstreamDefinition{
				{
					Name:      "alt-backend",
					BasePath:  "/api/v2",
					Timeout:   &model.UpstreamTimeout{Connect: "5s"},
					Upstreams: []model.UpstreamBackend{{URL: "http://alt:9090", Weight: &weight}},
				},
			},
			Operations: []model.Operation{
				{Request: &model.OperationRequest{Method: "GET", Path: "/x"}},
			},
		},
	}

	d, err := util.BuildAPIDeploymentYAML(apiModel)
	if err != nil {
		t.Fatalf("BuildAPIDeploymentYAML() error = %v", err)
	}
	if len(d.Spec.UpstreamDefinitions) != 1 {
		t.Fatalf("expected 1 upstreamDefinition, got %d", len(d.Spec.UpstreamDefinitions))
	}
	got := d.Spec.UpstreamDefinitions[0]
	if got.Name != "alt-backend" || got.BasePath != "/api/v2" {
		t.Errorf("got name=%q basePath=%q, want alt-backend // /api/v2", got.Name, got.BasePath)
	}
	if got.Timeout == nil || got.Timeout.Connect != "5s" {
		t.Errorf("Timeout = %+v, want connect 5s", got.Timeout)
	}
	if len(got.Upstreams) != 1 || got.Upstreams[0].URL != "http://alt:9090" ||
		got.Upstreams[0].Weight == nil || *got.Upstreams[0].Weight != 80 {
		t.Errorf("Upstreams = %+v, want url http://alt:9090 weight 80", got.Upstreams)
	}

	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal error = %v", err)
	}
	if !strings.Contains(string(out), "upstreamDefinitions:") || !strings.Contains(string(out), "alt-backend") {
		t.Errorf("emitted YAML missing upstreamDefinitions/alt-backend:\n%s", out)
	}
}

// TestBuildAPIDeploymentYAML_EmitsPerOpUpstreamRef verifies a per-operation upstream ref is
// emitted into the operation in the deployment YAML.
func TestBuildAPIDeploymentYAML_EmitsPerOpUpstreamRef(t *testing.T) {
	util := &APIUtil{}
	ctx := "/test"
	apiModel := &model.API{
		Name:    "PerOp API",
		Handle:  "perop-api",
		Version: "v1.0",
		Kind:    constants.RestApi,
		Configuration: model.RestAPIConfig{
			Context:  &ctx,
			Upstream: model.UpstreamConfig{Main: &model.UpstreamEndpoint{URL: "http://main:8080"}},
			Operations: []model.Operation{
				{Request: &model.OperationRequest{
					Method:   "GET",
					Path:     "/whoami",
					Upstream: &model.OperationUpstream{Main: &model.OperationUpstreamTarget{Ref: "alt-backend"}},
				}},
			},
		},
	}

	d, err := util.BuildAPIDeploymentYAML(apiModel)
	if err != nil {
		t.Fatalf("BuildAPIDeploymentYAML() error = %v", err)
	}
	if len(d.Spec.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(d.Spec.Operations))
	}
	up := d.Spec.Operations[0].Upstream
	if up == nil || up.Main == nil {
		t.Fatalf("expected per-op upstream.main, got %+v", up)
	}
	if up.Main.Ref != "alt-backend" {
		t.Errorf("per-op main ref = %q, want alt-backend", up.Main.Ref)
	}

	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal error = %v", err)
	}
	if !strings.Contains(string(out), "ref: alt-backend") {
		t.Errorf("emitted YAML missing per-op ref:\n%s", out)
	}
}

// TestAPIYAMLDataToRESTAPI_PreservesPoolAndPerOp guards the YAML->model round-trip (GAP-3).
func TestAPIYAMLDataToRESTAPI_PreservesPoolAndPerOp(t *testing.T) {
	util := &APIUtil{}
	yamlData := &dto.APIYAMLData{
		DisplayName: "x",
		Context:     "/x",
		Version:     "v1",
		UpstreamDefinitions: []dto.UpstreamDefinitionYAML{
			{Name: "alt-backend", BasePath: "/alternate", Upstreams: []dto.UpstreamBackendYAML{{URL: "http://b:8080"}}},
		},
		Operations: []api.OperationRequest{
			{Method: api.OperationRequestMethodGET, Path: "/whoami",
				Upstream: &api.OperationUpstream{Main: &api.OperationUpstreamTarget{Ref: "alt-backend"}}},
		},
	}
	rest := util.APIYAMLDataToRESTAPI(yamlData)
	if rest.UpstreamDefinitions == nil || len(*rest.UpstreamDefinitions) != 1 || (*rest.UpstreamDefinitions)[0].Name != "alt-backend" {
		t.Errorf("pool dropped in APIYAMLDataToRESTAPI: %+v", rest.UpstreamDefinitions)
	}
	if rest.Operations == nil || len(*rest.Operations) != 1 {
		t.Fatalf("operations missing")
	}
	up := (*rest.Operations)[0].Request.Upstream
	if up == nil || up.Main == nil || up.Main.Ref != "alt-backend" {
		t.Errorf("per-op upstream dropped in APIYAMLDataToRESTAPI: %+v", up)
	}
}

// TestMergeRESTAPIDetails_PreservesPoolAndOverlaysPerOp guards the OpenAPI overlay (GAP-2): the
// pool must survive and the user's per-op upstream must overlay onto the extracted operations.
func TestMergeRESTAPIDetails_PreservesPoolAndOverlaysPerOp(t *testing.T) {
	util := &APIUtil{}
	pool := &[]api.ReusableUpstream{{Name: "alt-backend"}}
	userAPI := &api.RESTAPI{
		Name:                "x",
		UpstreamDefinitions: pool,
		Operations: &[]api.Operation{
			{Request: api.OperationRequest{Method: api.OperationRequestMethodGET, Path: "/whoami",
				Upstream: &api.OperationUpstream{Main: &api.OperationUpstreamTarget{Ref: "alt-backend"}}}},
		},
	}
	extractedAPI := &api.RESTAPI{
		Name: "x",
		Operations: &[]api.Operation{
			{Request: api.OperationRequest{Method: api.OperationRequestMethodGET, Path: "/whoami"}},
			{Request: api.OperationRequest{Method: api.OperationRequestMethodGET, Path: "/ping"}},
		},
	}
	merged := util.MergeRESTAPIDetails(userAPI, extractedAPI)
	if merged.UpstreamDefinitions == nil || len(*merged.UpstreamDefinitions) != 1 {
		t.Errorf("pool dropped in MergeRESTAPIDetails: %+v", merged.UpstreamDefinitions)
	}
	if merged.Operations == nil || len(*merged.Operations) != 2 {
		t.Fatalf("expected 2 merged operations, got %v", merged.Operations)
	}
	var whoami, ping *api.OperationUpstream
	for _, op := range *merged.Operations {
		switch op.Request.Path {
		case "/whoami":
			whoami = op.Request.Upstream
		case "/ping":
			ping = op.Request.Upstream
		}
	}
	if whoami == nil || whoami.Main == nil || whoami.Main.Ref != "alt-backend" {
		t.Errorf("/whoami per-op upstream not overlaid: %+v", whoami)
	}
	if ping != nil {
		t.Errorf("/ping should have no per-op upstream, got %+v", ping)
	}
}

// TestBuildAPIDeploymentYAML_FullPerOpRefContract asserts the complete deployment-YAML shape the
// gateway must accept for the canonical scenario: a reusable pool, an API-level ref, a per-op ref
// override, and a fallback operation. (A live platform->gateway e2e belongs in the gateway IT.)
func TestBuildAPIDeploymentYAML_FullPerOpRefContract(t *testing.T) {
	util := &APIUtil{}
	ctx := "/test"
	apiModel := &model.API{
		Name:    "Full API",
		Handle:  "full-api",
		Version: "v1.0",
		Kind:    constants.RestApi,
		Configuration: model.RestAPIConfig{
			Context:  &ctx,
			Upstream: model.UpstreamConfig{Main: &model.UpstreamEndpoint{Ref: "alt-backend"}},
			UpstreamDefinitions: []model.UpstreamDefinition{
				{Name: "alt-backend", BasePath: "/alternate", Upstreams: []model.UpstreamBackend{{URL: "http://alt:9090"}}},
			},
			Operations: []model.Operation{
				{Request: &model.OperationRequest{Method: "GET", Path: "/whoami",
					Upstream: &model.OperationUpstream{Main: &model.OperationUpstreamTarget{Ref: "alt-backend"}}}},
				{Request: &model.OperationRequest{Method: "GET", Path: "/ping"}},
			},
		},
	}
	d, err := util.BuildAPIDeploymentYAML(apiModel)
	if err != nil {
		t.Fatalf("BuildAPIDeploymentYAML() error = %v", err)
	}
	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal error = %v", err)
	}

	// Round-trip back into the typed deployment DTO and assert structure (not just
	// substrings) so a ref attached to the wrong operation can't slip through.
	var got dto.APIDeploymentYAML
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("yaml.Unmarshal error = %v\n%s", err, out)
	}
	spec := got.Spec

	if len(spec.UpstreamDefinitions) != 1 {
		t.Fatalf("expected 1 upstreamDefinition, got %d:\n%s", len(spec.UpstreamDefinitions), out)
	}
	def := spec.UpstreamDefinitions[0]
	if def.Name != "alt-backend" || def.BasePath != "/alternate" {
		t.Errorf("pool definition mismatch: %+v", def)
	}
	if len(def.Upstreams) != 1 || def.Upstreams[0].URL != "http://alt:9090" {
		t.Errorf("pool backends mismatch: %+v", def.Upstreams)
	}

	// Per-op ref must be attached to /whoami, and /ping must carry no upstream override.
	byPath := make(map[string]api.OperationRequest, len(spec.Operations))
	for _, op := range spec.Operations {
		byPath[op.Path] = op
	}
	whoami, ok := byPath["/whoami"]
	if !ok {
		t.Fatalf("operation /whoami missing from emitted YAML:\n%s", out)
	}
	if whoami.Upstream == nil || whoami.Upstream.Main == nil || whoami.Upstream.Main.Ref != "alt-backend" {
		t.Errorf("/whoami should ref alt-backend, got %+v", whoami.Upstream)
	}
	ping, ok := byPath["/ping"]
	if !ok {
		t.Fatalf("operation /ping missing from emitted YAML:\n%s", out)
	}
	if ping.Upstream != nil {
		t.Errorf("/ping must not inherit a per-op upstream, got %+v", ping.Upstream)
	}
}
