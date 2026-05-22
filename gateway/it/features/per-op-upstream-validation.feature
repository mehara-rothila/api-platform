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

@per-op-upstream-validation
Feature: Per-Operation Upstream Validation
  As an API developer
  I want malformed per-operation upstream configurations to be rejected
  So that invalid APIs cannot be deployed

  Background:
    Given the gateway services are running

  Scenario: Empty per-op upstream wrapper is rejected
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-val-empty-api-v1.0
      spec:
        displayName: Per-Op-Val-Empty-API
        version: v1.0
        context: /per-op-val-empty/$version
        vhosts:
          main: per-op-val-empty-main.local
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream: {}
      """
    Then the response should be a client error
    And the response should be valid JSON
    And the JSON response field "status" should be "error"
    And the response body should contain "At least one of 'main' or 'sandbox' must be set"

  Scenario: Per-op target with both url and ref is rejected
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-val-both-api-v1.0
      spec:
        displayName: Per-Op-Val-Both-API
        version: v1.0
        context: /per-op-val-both/$version
        vhosts:
          main: per-op-val-both-main.local
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream:
              main:
                url: http://sample-backend:9080
                ref: some-ref
      """
    Then the response should be a client error
    And the response should be valid JSON
    And the JSON response field "status" should be "error"
    And the response body should contain "Specify exactly one of 'url' or 'ref'"

  Scenario: Per-op ref to non-existent upstream definition is rejected
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-val-missing-ref-api-v1.0
      spec:
        displayName: Per-Op-Val-Missing-Ref-API
        version: v1.0
        context: /per-op-val-missing-ref/$version
        vhosts:
          main: per-op-val-missing-ref-main.local
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream:
              main:
                ref: does-not-exist
      """
    Then the response should be a client error
    And the response should be valid JSON
    And the JSON response field "status" should be "error"
    And the response body should contain "Referenced upstream definition 'does-not-exist' not found"

  Scenario: Invalid URL scheme is rejected
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-val-scheme-api-v1.0
      spec:
        displayName: Per-Op-Val-Scheme-API
        version: v1.0
        context: /per-op-val-scheme/$version
        vhosts:
          main: per-op-val-scheme-main.local
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream:
              main:
                url: ftp://bogus
      """
    Then the response should be a client error
    And the response should be valid JSON
    And the JSON response field "status" should be "error"
    And the response body should contain "Upstream URL must use http or https scheme"

  Scenario: Empty per-op leaf is rejected
    Given I authenticate using basic auth as "admin"
    When I deploy this API configuration:
      """
      apiVersion: gateway.api-platform.wso2.com/v1alpha1
      kind: RestApi
      metadata:
        name: per-op-val-empty-leaf-api-v1.0
      spec:
        displayName: Per-Op-Val-Empty-Leaf-API
        version: v1.0
        context: /per-op-val-empty-leaf/$version
        vhosts:
          main: per-op-val-empty-leaf-main.local
        upstream:
          main:
            url: http://sample-backend:9080
        operations:
          - method: GET
            path: /users
            upstream:
              main: {}
      """
    Then the response should be a client error
    And the response should be valid JSON
    And the JSON response field "status" should be "error"
    And the response body should contain "Must specify either 'url' or 'ref'"
