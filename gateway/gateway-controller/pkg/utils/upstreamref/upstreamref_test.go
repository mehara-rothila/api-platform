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

package upstreamref

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api "github.com/wso2/api-platform/gateway/gateway-controller/pkg/api/management"
)

func TestFindByName_Found(t *testing.T) {
	defs := &[]api.UpstreamDefinition{
		{Name: "users-svc"},
		{Name: "orders-svc"},
	}
	def, err := FindByName("orders-svc", defs)
	require.NoError(t, err)
	require.NotNil(t, def)
	assert.Equal(t, "orders-svc", def.Name)
}

func TestFindByName_TrimsWhitespace(t *testing.T) {
	defs := &[]api.UpstreamDefinition{{Name: "users-svc"}}
	def, err := FindByName("  users-svc  ", defs)
	require.NoError(t, err)
	assert.Equal(t, "users-svc", def.Name)
}

func TestFindByName_EmptyRef(t *testing.T) {
	defs := &[]api.UpstreamDefinition{{Name: "users-svc"}}
	_, err := FindByName("", defs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestFindByName_WhitespaceRef(t *testing.T) {
	defs := &[]api.UpstreamDefinition{{Name: "users-svc"}}
	_, err := FindByName("   ", defs)
	require.Error(t, err)
}

func TestFindByName_NilDefs(t *testing.T) {
	_, err := FindByName("users-svc", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no definitions provided")
}

func TestFindByName_EmptyDefs(t *testing.T) {
	defs := &[]api.UpstreamDefinition{}
	_, err := FindByName("users-svc", defs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no definitions provided")
}

func TestFindByName_NotFound(t *testing.T) {
	defs := &[]api.UpstreamDefinition{{Name: "users-svc"}}
	_, err := FindByName("orders-svc", defs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindByName_ReturnsStablePointer(t *testing.T) {
	defs := &[]api.UpstreamDefinition{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	got, err := FindByName("b", defs)
	require.NoError(t, err)
	assert.Same(t, &(*defs)[1], got, "must return pointer into the slice, not a copy of a loop variable")
}

func TestParseConnectTimeout_NilInput(t *testing.T) {
	d, err := ParseConnectTimeout(nil)
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestParseConnectTimeout_EmptyString(t *testing.T) {
	empty := ""
	d, err := ParseConnectTimeout(&empty)
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestParseConnectTimeout_WhitespaceOnly(t *testing.T) {
	ws := "   "
	d, err := ParseConnectTimeout(&ws)
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestParseConnectTimeout_Valid(t *testing.T) {
	v := "5s"
	d, err := ParseConnectTimeout(&v)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, 5*time.Second, *d)
}

func TestParseConnectTimeout_ValidMilliseconds(t *testing.T) {
	v := "500ms"
	d, err := ParseConnectTimeout(&v)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, 500*time.Millisecond, *d)
}

func TestParseConnectTimeout_Malformed(t *testing.T) {
	v := "abc"
	_, err := ParseConnectTimeout(&v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout format")
}

func TestParseConnectTimeout_NoUnit(t *testing.T) {
	v := "30"
	_, err := ParseConnectTimeout(&v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout format")
}

func TestParseConnectTimeout_Zero(t *testing.T) {
	v := "0s"
	_, err := ParseConnectTimeout(&v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

func TestParseConnectTimeout_Negative(t *testing.T) {
	v := "-5s"
	_, err := ParseConnectTimeout(&v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

func TestResolveConnectTimeout_NilDef(t *testing.T) {
	d, err := ResolveConnectTimeout(nil)
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestResolveConnectTimeout_NoTimeoutBlock(t *testing.T) {
	def := &api.UpstreamDefinition{Name: "x"}
	d, err := ResolveConnectTimeout(def)
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestResolveConnectTimeout_ValidConnect(t *testing.T) {
	v := "3s"
	def := &api.UpstreamDefinition{
		Name:    "x",
		Timeout: &api.UpstreamTimeout{Connect: &v},
	}
	d, err := ResolveConnectTimeout(def)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, 3*time.Second, *d)
}

func TestResolveConnectTimeout_InvalidConnect(t *testing.T) {
	v := "-2s"
	def := &api.UpstreamDefinition{
		Name:    "x",
		Timeout: &api.UpstreamTimeout{Connect: &v},
	}
	_, err := ResolveConnectTimeout(def)
	require.Error(t, err)
}
