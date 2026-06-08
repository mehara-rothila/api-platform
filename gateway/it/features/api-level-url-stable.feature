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

@api-level-url-stable
Feature: API-Level Upstream URL-Stable Cluster Naming
  As an API developer
  I want API-level main and sandbox upstream URL edits to propagate as
  endpoint updates rather than cluster recreates
  So that URL changes do not drain in-flight connections

  Background:
    Given the gateway services are running

  Scenario: API-level main upstream URL update routes to new backend (URL-stable cluster naming)
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: api-level-eds-main-api-v1.0
      spec:
        displayName: API-Level-EDS-Main-API
        version: v1.0
        context: /api-level-eds-main/$version
        vhosts:
          main: api-level-eds-main.local
        upstream:
          main:
            url: http://sample-backend:9080/version-a
        operations:
          - method: GET
            path: /endpoint
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/api-level-eds-main/v1.0/endpoint" to be ready with host "api-level-eds-main.local"

    When I clear all headers
    And I set request host to "api-level-eds-main.local"
    And I send a GET request to "http://localhost:8080/api-level-eds-main/v1.0/endpoint"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/version-a/endpoint"

    Given I authenticate using basic auth as "admin"
    When I update the API "api-level-eds-main-api-v1.0" with this configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: api-level-eds-main-api-v1.0
      spec:
        displayName: API-Level-EDS-Main-API
        version: v1.0
        context: /api-level-eds-main/$version
        vhosts:
          main: api-level-eds-main.local
        upstream:
          main:
            url: http://sample-backend:9080/version-b
        operations:
          - method: GET
            path: /endpoint
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/api-level-eds-main/v1.0/endpoint" to be ready with host "api-level-eds-main.local"

    When I clear all headers
    And I set request host to "api-level-eds-main.local"
    And I send a GET request to "http://localhost:8080/api-level-eds-main/v1.0/endpoint"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/version-b/endpoint"

    Given I authenticate using basic auth as "admin"
    When I delete the API "api-level-eds-main-api-v1.0"
    Then the response should be successful

  Scenario: API-level sandbox upstream URL update routes to new backend
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: api-level-eds-sandbox-api-v1.0
      spec:
        displayName: API-Level-EDS-Sandbox-API
        version: v1.0
        context: /api-level-eds-sandbox/$version
        vhosts:
          main: api-level-eds-sandbox-main.local
          sandbox: api-level-eds-sandbox-sb.local
        upstream:
          main:
            url: http://sample-backend:9080/api-main
          sandbox:
            url: http://sample-backend:9080/sandbox-a
        operations:
          - method: GET
            path: /endpoint
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/api-level-eds-sandbox/v1.0/endpoint" to be ready with host "api-level-eds-sandbox-sb.local"

    When I clear all headers
    And I set request host to "api-level-eds-sandbox-sb.local"
    And I send a GET request to "http://localhost:8080/api-level-eds-sandbox/v1.0/endpoint"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/sandbox-a/endpoint"

    Given I authenticate using basic auth as "admin"
    When I update the API "api-level-eds-sandbox-api-v1.0" with this configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: api-level-eds-sandbox-api-v1.0
      spec:
        displayName: API-Level-EDS-Sandbox-API
        version: v1.0
        context: /api-level-eds-sandbox/$version
        vhosts:
          main: api-level-eds-sandbox-main.local
          sandbox: api-level-eds-sandbox-sb.local
        upstream:
          main:
            url: http://sample-backend:9080/api-main
          sandbox:
            url: http://sample-backend:9080/sandbox-b
        operations:
          - method: GET
            path: /endpoint
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/api-level-eds-sandbox/v1.0/endpoint" to be ready with host "api-level-eds-sandbox-sb.local"

    When I clear all headers
    And I set request host to "api-level-eds-sandbox-sb.local"
    And I send a GET request to "http://localhost:8080/api-level-eds-sandbox/v1.0/endpoint"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/sandbox-b/endpoint"

    Given I authenticate using basic auth as "admin"
    When I delete the API "api-level-eds-sandbox-api-v1.0"
    Then the response should be successful

  Scenario: API-level upstream with cluster_header routing (default upstream cluster resolves correctly)
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: api-level-eds-default-api-v1.0
      spec:
        displayName: API-Level-EDS-Default-API
        version: v1.0
        context: /api-level-eds-default/$version
        vhosts:
          main: api-level-eds-default.local
        upstreamDefinitions:
          - name: backend-default
            basePath: /api-main
            upstreams:
              - url: http://sample-backend:9080
        upstream:
          main:
            ref: backend-default
        operations:
          - method: GET
            path: /endpoint
      """
    Then the response should be successful
    And I wait for the endpoint "http://localhost:8080/api-level-eds-default/v1.0/endpoint" to be ready with host "api-level-eds-default.local"

    When I clear all headers
    And I set request host to "api-level-eds-default.local"
    And I send a GET request to "http://localhost:8080/api-level-eds-default/v1.0/endpoint"
    Then the response should be successful
    And the response should be valid JSON
    And the JSON response field "path" should be "/api-main/endpoint"

    Given I authenticate using basic auth as "admin"
    When I delete the API "api-level-eds-default-api-v1.0"
    Then the response should be successful
