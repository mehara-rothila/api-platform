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

package service

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"platform-api/src/api"
	"platform-api/src/internal/constants"
	"platform-api/src/internal/database"
	"platform-api/src/internal/dto"
	"platform-api/src/internal/model"
	"platform-api/src/internal/repository"
	"platform-api/src/internal/utils"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// mockAPIRepository is a mock implementation of the APIRepository interface
type mockAPIRepository struct {
	repository.APIRepository // Embed interface for unimplemented methods

	// Mock behavior configuration
	handleExistsResult      bool
	handleExistsError       error
	nameVersionExistsResult bool
	nameVersionExistsError  error

	// Call tracking for verification
	lastExcludeHandle string
}

func (m *mockAPIRepository) CheckAPIExistsByHandleInOrganization(handle, orgUUID string) (bool, error) {
	return m.handleExistsResult, m.handleExistsError
}

func (m *mockAPIRepository) CheckAPIExistsByNameAndVersionInOrganization(name, version, orgUUID, excludeHandle string) (bool, error) {
	m.lastExcludeHandle = excludeHandle // Track for verification
	return m.nameVersionExistsResult, m.nameVersionExistsError
}

// TestValidateUpdateAPIRequest tests the validateUpdateAPIRequest method
func TestValidateUpdateAPIRequest(t *testing.T) {
	tests := []struct {
		name                      string
		existingAPI               *model.API
		req                       *api.UpdateRESTAPIRequest
		mockHandleExists          bool
		mockHandleError           error
		mockNameVersionExists     bool
		mockNameVersionError      error
		wantErr                   bool
		expectedErr               error
		expectedExcludeHandle     string
		verifyExcludeHandleCalled bool
	}{
		{
			name: "valid update - no changes",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:     &api.UpdateRESTAPIRequest{},
			wantErr: false,
		},
		{
			name: "name update - excludes current API handle from duplicate check",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:                       &api.UpdateRESTAPIRequest{Name: "Updated Name"},
			mockNameVersionExists:     false,
			wantErr:                   false,
			expectedExcludeHandle:     "my-api",
			verifyExcludeHandleCalled: true,
		},
		{
			name: "name update - conflict with different API",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:                   &api.UpdateRESTAPIRequest{Name: "Conflicting Name"},
			mockNameVersionExists: true,
			wantErr:               true,
			expectedErr:           constants.ErrAPINameVersionAlreadyExists,
		},
		{
			name: "invalid lifecycle state",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:         &api.UpdateRESTAPIRequest{LifeCycleStatus: statusPtr("INVALID_STATE")},
			wantErr:     true,
			expectedErr: constants.ErrInvalidLifecycleState,
		},
		{
			name: "invalid api type",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:         &api.UpdateRESTAPIRequest{Kind: ptr("INVALID_TYPE")},
			wantErr:     true,
			expectedErr: constants.ErrInvalidAPIType,
		},
		{
			name: "invalid transport",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:         &api.UpdateRESTAPIRequest{Transport: slicePtr([]string{"invalid"})},
			wantErr:     true,
			expectedErr: constants.ErrInvalidTransport,
		},
		{
			name: "valid lifecycle state",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:     &api.UpdateRESTAPIRequest{LifeCycleStatus: statusPtr("PUBLISHED")},
			wantErr: false,
		},
		{
			name: "valid api type",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:     &api.UpdateRESTAPIRequest{Kind: ptr("RestApi")},
			wantErr: false,
		},
		{
			name: "valid transport",
			existingAPI: &model.API{
				Handle:  "my-api",
				Version: "v1",
			},
			req:     &api.UpdateRESTAPIRequest{Transport: slicePtr([]string{"https"})},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAPIRepository{
				handleExistsResult:      tt.mockHandleExists,
				handleExistsError:       tt.mockHandleError,
				nameVersionExistsResult: tt.mockNameVersionExists,
				nameVersionExistsError:  tt.mockNameVersionError,
			}

			service := &APIService{
				apiRepo: mock,
			}

			err := service.validateUpdateAPIRequest(tt.existingAPI, tt.req, "test-org-uuid")

			if (err != nil) != tt.wantErr {
				t.Errorf("validateUpdateAPIRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.expectedErr != nil {
				if err != tt.expectedErr {
					t.Errorf("validateUpdateAPIRequest() error = %v, expectedErr %v", err, tt.expectedErr)
				}
			}

			if tt.verifyExcludeHandleCalled {
				if mock.lastExcludeHandle != tt.expectedExcludeHandle {
					t.Errorf("excludeHandle = %q, want %q", mock.lastExcludeHandle, tt.expectedExcludeHandle)
				}
			}
		})
	}
}

// TestValidateCreateAPIRequest tests the validateCreateAPIRequest method
func TestValidateCreateAPIRequest(t *testing.T) {
	projectID := openapi_types.UUID(uuid.MustParse("11111111-1111-1111-1111-111111111111"))

	tests := []struct {
		name                     string
		req                      *api.CreateRESTAPIRequest
		mockHandleExists         bool
		mockHandleError          error
		mockNameVersionExists    bool
		mockNameVersionError     error
		wantErr                  bool
		expectedErr              error
		errContains              string
		verifyExcludeHandleEmpty bool
		expectedExcludeHandle    string
	}{
		{
			name: "valid create request",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Upstream:  api.Upstream{},
			},
			mockNameVersionExists:    false,
			wantErr:                  false,
			verifyExcludeHandleEmpty: true,
			expectedExcludeHandle:    "",
		},
		{
			name: "handle already exists",
			req: &api.CreateRESTAPIRequest{
				Id:        ptr("my-handle"),
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Upstream:  api.Upstream{},
			},
			mockHandleExists: true,
			wantErr:          true,
			expectedErr:      constants.ErrHandleExists,
		},
		{
			name: "name version already exists",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Upstream:  api.Upstream{},
			},
			mockNameVersionExists:    true,
			wantErr:                  true,
			expectedErr:              constants.ErrAPINameVersionAlreadyExists,
			verifyExcludeHandleEmpty: true,
			expectedExcludeHandle:    "",
		},
		{
			name: "missing name",
			req: &api.CreateRESTAPIRequest{
				Name:      "",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Upstream:  api.Upstream{},
			},
			wantErr:     true,
			expectedErr: constants.ErrInvalidAPIName,
		},
		{
			name: "missing project id",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: openapi_types.UUID{},
				Upstream:  api.Upstream{},
			},
			wantErr:     true,
			errContains: "project id is required",
		},
		{
			name: "invalid context",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "invalid",
				Version:   "v1",
				ProjectId: projectID,
				Upstream:  api.Upstream{},
			},
			wantErr:     true,
			expectedErr: constants.ErrInvalidAPIContext,
		},
		{
			name: "invalid version",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "",
				ProjectId: projectID,
				Upstream:  api.Upstream{},
			},
			wantErr:     true,
			expectedErr: constants.ErrInvalidAPIVersion,
		},
		{
			name: "invalid lifecycle state",
			req: &api.CreateRESTAPIRequest{
				Name:            "Test API",
				Context:         "/test",
				Version:         "v1",
				ProjectId:       projectID,
				LifeCycleStatus: createStatusPtr("INVALID_STATE"),
				Upstream:        api.Upstream{},
			},
			wantErr:     true,
			expectedErr: constants.ErrInvalidLifecycleState,
		},
		{
			name: "invalid api type",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Kind:      ptr("INVALID_TYPE"),
				Upstream:  api.Upstream{},
			},
			wantErr:     true,
			expectedErr: constants.ErrInvalidAPIType,
		},
		{
			name: "invalid transport",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Transport: slicePtr([]string{"invalid"}),
				Upstream:  api.Upstream{},
			},
			wantErr:     true,
			expectedErr: constants.ErrInvalidTransport,
		},
		{
			name: "valid lifecycle state",
			req: &api.CreateRESTAPIRequest{
				Name:            "Test API",
				Context:         "/test",
				Version:         "v1",
				ProjectId:       projectID,
				LifeCycleStatus: createStatusPtr("PUBLISHED"),
				Upstream:        api.Upstream{},
			},
			mockNameVersionExists:    false,
			wantErr:                  false,
			verifyExcludeHandleEmpty: true,
			expectedExcludeHandle:    "",
		},
		{
			name: "valid api type",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Kind:      ptr("RestApi"),
				Upstream:  api.Upstream{},
			},
			mockNameVersionExists:    false,
			wantErr:                  false,
			verifyExcludeHandleEmpty: true,
			expectedExcludeHandle:    "",
		},
		{
			name: "valid transport",
			req: &api.CreateRESTAPIRequest{
				Name:      "Test API",
				Context:   "/test",
				Version:   "v1",
				ProjectId: projectID,
				Transport: slicePtr([]string{"https"}),
				Upstream:  api.Upstream{},
			},
			mockNameVersionExists:    false,
			wantErr:                  false,
			verifyExcludeHandleEmpty: true,
			expectedExcludeHandle:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAPIRepository{
				handleExistsResult:      tt.mockHandleExists,
				handleExistsError:       tt.mockHandleError,
				nameVersionExistsResult: tt.mockNameVersionExists,
				nameVersionExistsError:  tt.mockNameVersionError,
			}

			service := &APIService{
				apiRepo: mock,
			}

			err := service.validateCreateAPIRequest(tt.req, "test-org-uuid")

			if (err != nil) != tt.wantErr {
				t.Errorf("validateCreateAPIRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.expectedErr != nil && err != tt.expectedErr {
					t.Errorf("validateCreateAPIRequest() error = %v, expectedErr %v", err, tt.expectedErr)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateCreateAPIRequest() error = %v, should contain %v", err, tt.errContains)
				}
			}

			if tt.verifyExcludeHandleEmpty {
				if mock.lastExcludeHandle != tt.expectedExcludeHandle {
					t.Errorf("excludeHandle = %q, want %q", mock.lastExcludeHandle, tt.expectedExcludeHandle)
				}
			}
		})
	}
}

func TestApplyAPIUpdatesUpdatesPolicies(t *testing.T) {
	service := &APIService{
		apiRepo: &mockAPIRepository{},
		apiUtil: &utils.APIUtil{},
	}

	condition := "request.path == '/pets'"
	params := map[string]interface{}{"limit": 10}
	newPolicies := []api.Policy{
		{
			ExecutionCondition: &condition,
			Name:               "rate-limit",
			Params:             &params,
			Version:            "v1",
		},
	}
	updatedPolicies := []api.Policy{
		{
			ExecutionCondition: &condition,
			Name:               "rate-limit",
			Params:             &params,
			Version:            "v1",
		},
	}

	existing := &model.API{
		Handle:    "pets-api",
		ProjectID: "11111111-1111-1111-1111-111111111111",
		Version:   "v1",
		Configuration: model.RestAPIConfig{
			Policies: []model.Policy{
				{Name: "legacy-policy", Version: "v1"},
			},
		},
	}

	updated, err := service.applyAPIUpdates(existing, &api.UpdateRESTAPIRequest{Policies: &newPolicies}, "org-1")
	if err != nil {
		t.Fatalf("applyAPIUpdates() error = %v", err)
	}

	if updated.Policies == nil {
		t.Fatalf("updated policies = nil, want %v", updatedPolicies)
	}
	if !reflect.DeepEqual(*updated.Policies, updatedPolicies) {
		t.Errorf("updated policies = %v, want %v", *updated.Policies, updatedPolicies)
	}
}

// Helper functions

// ptr creates a string pointer
func ptr(s string) *string {
	return &s
}

// slicePtr creates a string slice pointer
func slicePtr(s []string) *[]string {
	return &s
}

func statusPtr(s string) *api.RESTAPILifeCycleStatus {
	status := api.RESTAPILifeCycleStatus(s)
	return &status
}

func createStatusPtr(s string) *api.CreateRESTAPIRequestLifeCycleStatus {
	status := api.CreateRESTAPIRequestLifeCycleStatus(s)
	return &status
}

// Note: contains() and findSubstring() helper functions are defined in gateway_test.go

// TestValidateUpstreamRefs verifies that API-level and per-operation upstream refs must name a
// declared upstreamDefinition, and that duplicate definition names are rejected.
func TestValidateUpstreamRefs(t *testing.T) {
	s := &APIService{}
	ru := func(name string) api.ReusableUpstream {
		r := api.ReusableUpstream{Name: name}
		r.Upstreams = append(r.Upstreams, struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{Url: "http://b:8080"})
		return r
	}
	defs := &[]api.ReusableUpstream{ru("alt-backend"), ru("foo")}
	opWithMainRef := func(ref string) *[]api.Operation {
		return &[]api.Operation{
			{Request: api.OperationRequest{
				Method:   api.OperationRequestMethodGET,
				Path:     "/x",
				Upstream: &api.OperationUpstream{Main: &api.OperationUpstreamTarget{Ref: ref}},
			}},
		}
	}

	t.Run("per-op main ref resolves", func(t *testing.T) {
		if err := s.validateUpstreamRefs(defs, api.Upstream{}, opWithMainRef("alt-backend")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
	t.Run("per-op sandbox ref resolves", func(t *testing.T) {
		ops := &[]api.Operation{
			{Request: api.OperationRequest{
				Method:   api.OperationRequestMethodGET,
				Path:     "/x",
				Upstream: &api.OperationUpstream{Sandbox: &api.OperationUpstreamTarget{Ref: "foo"}},
			}},
		}
		if err := s.validateUpstreamRefs(defs, api.Upstream{}, ops); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
	t.Run("per-op ref unresolved", func(t *testing.T) {
		if err := s.validateUpstreamRefs(defs, api.Upstream{}, opWithMainRef("missing")); err == nil {
			t.Error("expected error for undefined per-op ref")
		}
	})
	t.Run("API-level ref unresolved", func(t *testing.T) {
		ref := "nope"
		up := api.Upstream{Main: api.UpstreamDefinition{Ref: &ref}}
		if err := s.validateUpstreamRefs(defs, up, nil); err == nil {
			t.Error("expected error for undefined API-level ref")
		}
	})
	t.Run("duplicate definition names", func(t *testing.T) {
		dup := &[]api.ReusableUpstream{ru("x"), ru("x")}
		if err := s.validateUpstreamRefs(dup, api.Upstream{}, nil); err == nil {
			t.Error("expected error for duplicate definition names")
		}
	})
	t.Run("definition with zero upstreams rejected", func(t *testing.T) {
		bare := &[]api.ReusableUpstream{{Name: "bare"}}
		if err := s.validateUpstreamRefs(bare, api.Upstream{}, nil); err == nil {
			t.Error("expected error for definition with no upstreams")
		}
	})
	t.Run("non-first upstream with empty url rejected", func(t *testing.T) {
		d := api.ReusableUpstream{Name: "multi"}
		d.Upstreams = append(d.Upstreams,
			struct {
				Url    string `json:"url" yaml:"url"`
				Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
			}{Url: "http://b1:8080"},
			struct {
				Url    string `json:"url" yaml:"url"`
				Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
			}{Url: ""},
		)
		if err := s.validateUpstreamRefs(&[]api.ReusableUpstream{d}, api.Upstream{}, nil); err == nil {
			t.Error("expected error for empty url on a non-first upstream")
		}
	})
	mkDef := func(name, u string) api.ReusableUpstream {
		d := api.ReusableUpstream{Name: name}
		d.Upstreams = append(d.Upstreams, struct {
			Url    string `json:"url" yaml:"url"`
			Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
		}{Url: u})
		return d
	}
	wantErr := func(t *testing.T, defs []api.ReusableUpstream, ops *[]api.Operation, what string) {
		if err := s.validateUpstreamRefs(&defs, api.Upstream{}, ops); err == nil {
			t.Errorf("expected error for %s", what)
		}
	}
	t.Run("zero weight accepted", func(t *testing.T) {
		zero := 0
		d := mkDef("w", "http://b:8080")
		d.Upstreams[0].Weight = &zero
		if err := s.validateUpstreamRefs(&[]api.ReusableUpstream{d}, api.Upstream{}, nil); err != nil {
			t.Errorf("zero weight should be valid (0..100), got %v", err)
		}
	})
	t.Run("weight over 100 rejected", func(t *testing.T) {
		over := 105
		d := mkDef("w", "http://b:8080")
		d.Upstreams[0].Weight = &over
		wantErr(t, []api.ReusableUpstream{d}, nil, "weight > 100")
	})
	t.Run("def name bad chars rejected", func(t *testing.T) {
		wantErr(t, []api.ReusableUpstream{mkDef("bad name!", "http://b:8080")}, nil, "bad definition name")
	})
	t.Run("def name too long rejected", func(t *testing.T) {
		wantErr(t, []api.ReusableUpstream{mkDef(strings.Repeat("a", 101), "http://b:8080")}, nil, "definition name > 100")
	})
	t.Run("def url with path rejected", func(t *testing.T) {
		wantErr(t, []api.ReusableUpstream{mkDef("d", "http://b:8080/foo")}, nil, "url with path")
	})
	t.Run("def url bad scheme rejected", func(t *testing.T) {
		wantErr(t, []api.ReusableUpstream{mkDef("d", "ftp://b:8080")}, nil, "url bad scheme")
	})
	t.Run("def url no host rejected", func(t *testing.T) {
		wantErr(t, []api.ReusableUpstream{mkDef("d", "http://")}, nil, "url no host")
	})
	t.Run("per-op empty ref rejected", func(t *testing.T) {
		defs := []api.ReusableUpstream{mkDef("svc", "http://b:8080")}
		ops := []api.Operation{{Request: api.OperationRequest{Method: api.OperationRequestMethodGET, Path: "/x",
			Upstream: &api.OperationUpstream{Main: &api.OperationUpstreamTarget{Ref: ""}}}}}
		wantErr(t, defs, &ops, "empty per-op ref")
	})
	t.Run("per-op empty wrapper rejected", func(t *testing.T) {
		defs := []api.ReusableUpstream{mkDef("svc", "http://b:8080")}
		ops := []api.Operation{{Request: api.OperationRequest{Method: api.OperationRequestMethodGET, Path: "/x",
			Upstream: &api.OperationUpstream{}}}}
		wantErr(t, defs, &ops, "empty per-op upstream wrapper")
	})
	t.Run("bad http method rejected", func(t *testing.T) {
		bad := []api.Operation{{Request: api.OperationRequest{Method: api.OperationRequestMethod("FETCH"), Path: "/x"}}}
		if err := validateOperationMethods(&bad); err == nil {
			t.Error("expected error for invalid http method")
		}
	})
	t.Run("invalid timeout.connect rejected", func(t *testing.T) {
		bad := "5x"
		d := ru("t")
		d.Timeout = &api.UpstreamTimeout{Connect: &bad}
		if err := s.validateUpstreamRefs(&[]api.ReusableUpstream{d}, api.Upstream{}, nil); err == nil {
			t.Error("expected error for invalid timeout.connect")
		}
	})
	t.Run("non-positive timeout.connect rejected", func(t *testing.T) {
		for _, v := range []string{"0s", "-1s"} {
			d := ru("t")
			d.Timeout = &api.UpstreamTimeout{Connect: &v}
			if err := s.validateUpstreamRefs(&[]api.ReusableUpstream{d}, api.Upstream{}, nil); err == nil {
				t.Errorf("expected error for non-positive timeout.connect %q", v)
			}
		}
	})
	t.Run("valid timeout.connect accepted", func(t *testing.T) {
		good := "5s"
		d := ru("t")
		d.Timeout = &api.UpstreamTimeout{Connect: &good}
		if err := s.validateUpstreamRefs(&[]api.ReusableUpstream{d}, api.Upstream{}, nil); err != nil {
			t.Errorf("expected nil for valid timeout.connect, got %v", err)
		}
	})
	t.Run("non-canonical timeout.connect unit rejected", func(t *testing.T) {
		// time.ParseDuration accepts these, but the gateway only allows ms, s, m, h, so the
		// control plane must reject them too or the API saves and then fails to deploy.
		for _, v := range []string{"1h30m", "500ns", "500us"} {
			d := ru("t")
			d.Timeout = &api.UpstreamTimeout{Connect: &v}
			if err := s.validateUpstreamRefs(&[]api.ReusableUpstream{d}, api.Upstream{}, nil); err == nil {
				t.Errorf("expected error for non-canonical timeout.connect unit %q", v)
			}
		}
	})
	t.Run("no refs and no defs is valid", func(t *testing.T) {
		if err := s.validateUpstreamRefs(nil, api.Upstream{}, nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
}

// TestCreateRequestToRESTAPI_PreservesUpstreamDefinitions guards the create path (GAP-4).
func TestCreateRequestToRESTAPI_PreservesUpstreamDefinitions(t *testing.T) {
	s := &APIService{}
	pool := &[]api.ReusableUpstream{{Name: "alt-backend"}}
	req := &api.CreateRESTAPIRequest{Name: "x", UpstreamDefinitions: pool}
	rest := s.createRequestToRESTAPI(req, "h")
	if rest.UpstreamDefinitions == nil || len(*rest.UpstreamDefinitions) != 1 || (*rest.UpstreamDefinitions)[0].Name != "alt-backend" {
		t.Errorf("upstreamDefinitions dropped on create: %+v", rest.UpstreamDefinitions)
	}
}

// TestRestAPIToCreateRequest_PreservesUpstreamDefinitions guards the OpenAPI-import path (GAP-5).
func TestRestAPIToCreateRequest_PreservesUpstreamDefinitions(t *testing.T) {
	s := &APIService{}
	pool := &[]api.ReusableUpstream{{Name: "alt-backend"}}
	rest := &api.RESTAPI{Name: "x", UpstreamDefinitions: pool}
	req := s.restAPIToCreateRequest(rest)
	if req.UpstreamDefinitions == nil || len(*req.UpstreamDefinitions) != 1 {
		t.Errorf("upstreamDefinitions dropped in restAPIToCreateRequest: %+v", req.UpstreamDefinitions)
	}
}

// TestCreateRequestFromAPIYAMLData_PreservesPoolAndPerOp guards Git-project import (GAP-1).
func TestCreateRequestFromAPIYAMLData_PreservesPoolAndPerOp(t *testing.T) {
	s := &APIService{apiUtil: &utils.APIUtil{}}
	yamlData := &dto.APIYAMLData{
		DisplayName: "x",
		Context:     "/x",
		Version:     "v1",
		UpstreamDefinitions: []dto.UpstreamDefinitionYAML{
			{Name: "alt-backend", Upstreams: []dto.UpstreamBackendYAML{{URL: "http://b:8080"}}},
		},
		Operations: []api.OperationRequest{
			{Method: api.OperationRequestMethodGET, Path: "/whoami",
				Upstream: &api.OperationUpstream{Main: &api.OperationUpstreamTarget{Ref: "alt-backend"}}},
		},
	}
	req := s.createRequestFromAPIYAMLData(yamlData)
	if req.UpstreamDefinitions == nil || len(*req.UpstreamDefinitions) != 1 || (*req.UpstreamDefinitions)[0].Name != "alt-backend" {
		t.Errorf("pool dropped on project import: %+v", req.UpstreamDefinitions)
	}
	if req.Operations == nil || len(*req.Operations) != 1 {
		t.Fatalf("operations missing")
	}
	up := (*req.Operations)[0].Request.Upstream
	if up == nil || up.Main == nil || up.Main.Ref != "alt-backend" {
		t.Errorf("per-op upstream dropped on project import: %+v", up)
	}
}

// --- Service-level integration tests (real SQLite + real repositories) ---

const perOpITProjectUUID = "11111111-1111-1111-1111-111111111111"

// setupServiceWithRealDB boots the real APIService backed by a fresh SQLite database
// (real repositories, not mocks) and seeds an organization + project the API can belong
// to. It returns the service and the organization UUID.
func setupServiceWithRealDB(t *testing.T) (*APIService, string) {
	t.Helper()

	tmpDir := t.TempDir()
	sqlDB, err := sql.Open("sqlite3", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	db := &database.DB{DB: sqlDB}

	// Resolve the schema path relative to this test file so the helper does not depend
	// on the test's working directory.
	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Join(filepath.Dir(thisFile), "..", "database", "schema.sqlite.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	const orgUUID = "22222222-2222-2222-2222-222222222222"
	if _, err := db.Exec(
		`INSERT INTO organizations (uuid, handle, name, region, created_at, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		orgUUID, "test-org", "Test Org", "default"); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO projects (uuid, name, organization_uuid, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		perOpITProjectUUID, "Test Project", orgUUID, "integration test project"); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewAPIService(
		repository.NewAPIRepo(db),
		repository.NewProjectRepo(db),
		repository.NewOrganizationRepo(db),
		repository.NewGatewayRepo(db),
		repository.NewDeploymentRepo(db),
		repository.NewDevPortalRepository(db),
		repository.NewAPIPublicationRepository(db),
		repository.NewSubscriptionPlanRepo(db),
		repository.NewCustomPolicyRepo(db),
		nil, // gatewayEventsService: not exercised by create/read
		nil, // devPortalService: not exercised by create/read
		&utils.APIUtil{},
		logger,
	)
	return svc, orgUUID
}

// perOpReusableUpstream builds a single-backend reusable upstream definition.
func perOpReusableUpstream(name, basePath, url string) api.ReusableUpstream {
	r := api.ReusableUpstream{Name: name, BasePath: &basePath}
	r.Upstreams = append(r.Upstreams, struct {
		Url    string `json:"url" yaml:"url"`
		Weight *int   `json:"weight,omitempty" yaml:"weight,omitempty"`
	}{Url: url})
	return r
}

// perOpCreateRequest is a create request carrying a reusable upstream pool, an API-level
// direct URL, and a per-operation ref override on /whoami (with /ping left to the default).
func perOpCreateRequest() *api.CreateRESTAPIRequest {
	handle := "perop-svc-it-api"
	desc := "service-level integration test API"
	mainURL := "http://default-backend:8080"
	defs := []api.ReusableUpstream{perOpReusableUpstream("alt-backend", "/alternate", "http://alt-backend:9090")}
	ops := []api.Operation{
		{Request: api.OperationRequest{
			Method: api.OperationRequestMethodGET, Path: "/whoami",
			Upstream: &api.OperationUpstream{Main: &api.OperationUpstreamTarget{Ref: "alt-backend"}},
		}},
		{Request: api.OperationRequest{Method: api.OperationRequestMethodGET, Path: "/ping"}},
	}
	return &api.CreateRESTAPIRequest{
		Name:                "PerOp Svc IT API",
		Id:                  &handle,
		Description:         &desc,
		Context:             "/perop-svc-it",
		Version:             "1.0.0",
		ProjectId:           uuid.MustParse(perOpITProjectUUID),
		Upstream:            api.Upstream{Main: api.UpstreamDefinition{Url: &mainURL}},
		UpstreamDefinitions: &defs,
		Operations:          &ops,
	}
}

// assertPoolAndPerOp checks that a RESTAPI carries the reusable pool and that the per-op
// ref is attached to /whoami (and not inherited by /ping).
func assertPoolAndPerOp(t *testing.T, where string, a *api.RESTAPI) {
	t.Helper()
	if a.UpstreamDefinitions == nil || len(*a.UpstreamDefinitions) != 1 {
		t.Fatalf("%s: want 1 upstreamDefinition, got %+v", where, a.UpstreamDefinitions)
	}
	def := (*a.UpstreamDefinitions)[0]
	if def.Name != "alt-backend" {
		t.Errorf("%s: pool name mismatch: %+v", where, def)
	}
	if def.BasePath == nil || *def.BasePath != "/alternate" {
		t.Errorf("%s: pool basePath mismatch: %+v", where, def.BasePath)
	}
	if len(def.Upstreams) != 1 || def.Upstreams[0].Url != "http://alt-backend:9090" {
		t.Errorf("%s: pool backends mismatch: %+v", where, def.Upstreams)
	}

	if a.Operations == nil {
		t.Fatalf("%s: operations missing", where)
	}
	var whoami, ping *api.Operation
	for i := range *a.Operations {
		switch (*a.Operations)[i].Request.Path {
		case "/whoami":
			whoami = &(*a.Operations)[i]
		case "/ping":
			ping = &(*a.Operations)[i]
		}
	}
	if whoami == nil {
		t.Fatalf("%s: /whoami missing", where)
	}
	if whoami.Request.Upstream == nil || whoami.Request.Upstream.Main == nil ||
		whoami.Request.Upstream.Main.Ref != "alt-backend" {
		t.Errorf("%s: /whoami should ref alt-backend, got %+v", where, whoami.Request.Upstream)
	}
	if ping == nil {
		t.Fatalf("%s: /ping missing", where)
	}
	if ping.Request.Upstream != nil {
		t.Errorf("%s: /ping must not carry a per-op upstream, got %+v", where, ping.Request.Upstream)
	}
}

// TestAPIService_CreatePersistAndReadPerOpUpstream creates an API with a reusable pool
// and a per-operation ref through the real service + repositories, then reads it back,
// asserting the pool and the per-op override survive validation and the DB round-trip.
func TestAPIService_CreatePersistAndReadPerOpUpstream(t *testing.T) {
	svc, orgUUID := setupServiceWithRealDB(t)

	created, err := svc.CreateAPI(perOpCreateRequest(), orgUUID)
	if err != nil {
		t.Fatalf("CreateAPI: %v", err)
	}
	assertPoolAndPerOp(t, "create", created)

	read, err := svc.GetAPIByHandle("perop-svc-it-api", orgUUID)
	if err != nil {
		t.Fatalf("GetAPIByHandle: %v", err)
	}
	assertPoolAndPerOp(t, "read", read)
}

// TestAPIService_CreateRejectsUnresolvedPerOpRef ensures a per-operation ref that does
// not resolve to a declared upstreamDefinition is rejected with ErrInvalidUpstreamDefinition
// (which the handler maps to HTTP 400), not silently persisted.
func TestAPIService_CreateRejectsUnresolvedPerOpRef(t *testing.T) {
	svc, orgUUID := setupServiceWithRealDB(t)

	req := perOpCreateRequest()
	badHandle := "bad-ref-api"
	req.Id = &badHandle
	req.Context = "/perop-svc-it-bad"
	req.UpstreamDefinitions = nil // drop the pool so the per-op ref no longer resolves

	if _, err := svc.CreateAPI(req, orgUUID); err == nil {
		t.Fatal("expected error for unresolved per-op ref")
	} else if !errors.Is(err, constants.ErrInvalidUpstreamDefinition) {
		t.Errorf("want ErrInvalidUpstreamDefinition, got %v", err)
	}
}
