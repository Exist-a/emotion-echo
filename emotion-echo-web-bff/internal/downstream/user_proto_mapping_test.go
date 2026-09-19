// Package downstream — user_proto_mapping_test.go
//
// E2E-11 复查：补齐 proto↔types 映射的**直接单测**。
//
// 此前 fromProtoUserInfo 只被端到端（curl / user-svc 联调）覆盖，
// 它恰是头像链路的第 4 个丢弃点——漏映射时 HTTP 仍 200，只有 DB 里 avatar_url
// 为空才能发现。单测把这个映射钉在改动点上。
package downstream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromProtoUserInfo_MapsAllFields(t *testing.T) {
	u := fromProtoUserInfo(&emotionuser.UserInfo{
		Id:        42,
		Username:  "alice",
		Nickname:  "Alice",
		AvatarUrl: "http://localhost:9000/avatars/avatars/42-abc.png",
		CreatedAt: 1700000000,
	})
	require.NotNil(t, u)
	assert.Equal(t, int64(42), u.UserID)
	assert.Equal(t, "alice", u.Account)
	assert.Equal(t, "Alice", u.Nickname)
	assert.Equal(t, "http://localhost:9000/avatars/avatars/42-abc.png", u.AvatarURL,
		"E2E-11: proto 的 avatar_url 必须映射到 UserInfo.AvatarURL（否则 profile 恒无头像）")
	assert.Equal(t, int64(1700000000), u.CreatedAt,
		"E2E-11: proto 的 created_at 必须映射（否则前端只能拿到 time.Now()）")
}

func TestFromProtoUserInfo_Nil_ReturnsNil(t *testing.T) {
	assert.Nil(t, fromProtoUserInfo(nil))
}

func TestFromProtoUserInfo_EmptyStrings_NoPanic(t *testing.T) {
	u := fromProtoUserInfo(&emotionuser.UserInfo{})
	require.NotNil(t, u)
	assert.Equal(t, "", u.AvatarURL)
	assert.Equal(t, int64(0), u.CreatedAt)
}

// ============ HTTP fallback transport ============

// TestUserHTTPClient_GetMe_MapsAvatarURL 契约：
// BFF 的 HTTP transport（feature flag 回退路径）同样必须解析 avatarUrl/createdAt。
// 此前只测过 gRPC 路径，HTTP 路径零覆盖——两条路径字段映射漂移过一次
// （见 E4E-F-72 同类：HTTP 通了 gRPC 是桩）。
func TestUserHTTPClient_GetMe_MapsAvatarURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/users/me", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{
				"userId":    42,
				"account":   "alice",
				"nickname":  "Alice",
				"avatarUrl": "http://localhost:9000/avatars/avatars/42-abc.png",
				"createdAt": 1700000000,
			},
		})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL})
	require.NotNil(t, c)
	u, err := c.GetMe(WithUserID(t.Context(), 42))
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, int64(42), u.UserID)
	assert.Equal(t, "Alice", u.Nickname)
	assert.Equal(t, "http://localhost:9000/avatars/avatars/42-abc.png", u.AvatarURL,
		"HTTP transport 路径同样要解析 avatarUrl")
	assert.Equal(t, int64(1700000000), u.CreatedAt, "HTTP transport 路径同样要解析 createdAt")
}

// TestUserHTTPClient_UpdateMe_SendsAvatarURL 契约：
// HTTP transport 的 UpdateMe 请求体必须带 avatarUrl（gRPC 侧曾漏映射，见 E2E-11）。
func TestUserHTTPClient_UpdateMe_SendsAvatarURL(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{"userId": 42, "account": "alice", "avatarUrl": gotBody["avatarUrl"]},
		})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL})
	require.NotNil(t, c)
	avatar := "http://localhost:9000/avatars/avatars/42-new.png"
	_, err := c.UpdateMe(WithUserID(t.Context(), 42), UpdateProfileReq{AvatarURL: &avatar})
	require.NoError(t, err)
	assert.Equal(t, avatar, gotBody["avatarUrl"], "HTTP PATCH 请求体必须含 avatarUrl")
}
