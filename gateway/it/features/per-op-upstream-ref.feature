# --------------------------------------------------------------------
# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
# --------------------------------------------------------------------

@per-op-upstream-ref
Feature: Per-Operation Upstream Ref
  As an API developer
  I want per-operation upstream refs to resolve through upstreamDefinitions
  So that different operations can route to different backends

  Background:
    Given the gateway services are running

  Scenario: Per-operation main refs route to different backends
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-ref-api-v1.0
      spec:
        displayName: Per-Op-Ref-API
        version: v1.0
        context: /per-op/$version
        vhosts:
          main: per-op-main.local
        upstreamDefinitions:
          - name: users-svc
            upstreams:
              - url: http://sample-backend:9080/user-svc
          - name: orders-svc
            upstreams:
              - url: http://sample-backend:9080/order-svc
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream:
              main:
                ref: users-svc
          - method: GET
            path: /orders
            upstream:
              main:
                ref: orders-svc
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/per-op/v1.0/users" to be ready with host "per-op-main.local"

    When I clear all headers
    And I set request host to "per-op-main.local"
    And I send a GET request to "http://localhost:8080/per-op/v1.0/users"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/user-svc/users"

    When I clear all headers
    And I set request host to "per-op-main.local"
    And I send a GET request to "http://localhost:8080/per-op/v1.0/orders"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/order-svc/orders"

    Given I authenticate using basic auth as "admin"
    When I delete the API "per-op-ref-api-v1.0"
    Then the response should be successful

  Scenario: Per-operation sandbox ref routes independently of main
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-sandbox-ref-api-v1.0
      spec:
        displayName: Per-Op-Sandbox-Ref-API
        version: v1.0
        context: /per-op-sb/$version
        vhosts:
          main: per-op-sb-main.local
          sandbox: per-op-sb-sandbox.local
        upstreamDefinitions:
          - name: sandbox-users-svc
            upstreams:
              - url: http://sample-backend:9080/sandbox-user-svc
        upstream:
          main:
            url: http://sample-backend:9080
          sandbox:
            url: http://sample-backend:9080/sandbox
        operations:
          - method: GET
            path: /users
            upstream:
              sandbox:
                ref: sandbox-users-svc
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/per-op-sb/v1.0/users" to be ready with host "per-op-sb-main.local"

    When I clear all headers
    And I set request host to "per-op-sb-main.local"
    And I send a GET request to "http://localhost:8080/per-op-sb/v1.0/users"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/users"

    When I clear all headers
    And I set request host to "per-op-sb-sandbox.local"
    And I send a GET request to "http://localhost:8080/per-op-sb/v1.0/users"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/sandbox-user-svc/users"

    Given I authenticate using basic auth as "admin"
    When I delete the API "per-op-sandbox-ref-api-v1.0"
    Then the response should be successful

  Scenario: Per-operation ref to missing upstreamDefinition should fail
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-missing-ref-api-v1.0
      spec:
        displayName: Per-Op-Missing-Ref-API
        version: v1.0
        context: /per-op-missing/$version
        vhosts:
          main: per-op-missing-main.local
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream:
              main:
                ref: non-existent-upstream
      """
    Then the response should be a client error
    And the response should be valid JSON
    And the JSON response field "status" should be "error"
    And the response body should contain "Referenced upstream definition 'non-existent-upstream' not found"
