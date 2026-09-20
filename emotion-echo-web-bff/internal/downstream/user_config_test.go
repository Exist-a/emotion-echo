// Package downstream — user_config_test.go
//
// E2E-12 RED：UpdateProfileReq 无 Config 字段，前端 PATCH /users/me {"config":{...}}
// 的 config 被 Go json.Unmarshal 静默丢弃。
// 契约：UpdateProfileReq 必须包含 Config 字段，且 UpdateMe 必须将其序列化到请求体。
package downstream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserClient_UpdateMe_SendsConfig(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v1/users/me", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedBody))

		_ = json.NewEncoder(w).Encode(userWrapper{User: &UserInfo{
			UserID:   1,
			Account:  "echo",
			Nickname: "Echo",
			Config:   map[string]any{"fontSize": "18px", "theme": "dark"},
		}})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})

	config := map[string]any{"fontSize": "18px", "theme": "dark"}
	u, err := c.UpdateMe(WithUserID(context.Background(), 1), UpdateProfileReq{
		Config: &config,
	})
	require.NoError(t, err)
	require.NotNil(t, u)

	// 断言请求体包含 config
	assert.NotNil(t, receivedBody["config"], "请求体必须包含 config 字段")
	bodyConfig, ok := receivedBody["config"].(map[string]any)
	require.True(t, ok, "config 应为 map")
	assert.Equal(t, "18px", bodyConfig["fontSize"])
	assert.Equal(t, "dark", bodyConfig["theme"])

	// 断言响应包含 config
	assert.Equal(t, "18px", u.Config["fontSize"], "响应 UserInfo.Config 必须包含 fontSize")
	assert.Equal(t, "dark", u.Config["theme"], "响应 UserInfo.Config 必须包含 theme")
}

func TestUserClient_GetMe_ReturnsConfig(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/users/me", r.URL.Path)
		_ = json.NewEncoder(w).Encode(userWrapper{User: &UserInfo{
			UserID:   1,
			Account:  "echo",
			Nickname: "Echo",
			Config:   map[string]any{"fontSize": "small", "theme": "auto"},
		}})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	u, err := c.GetMe(WithUserID(context.Background(), 1))
	require.NoError(t, err)
	require.NotNil(t, u)

	assert.Equal(t, "small", u.Config["fontSize"], "GetMe 响应必须包含 config.fontSize")
	assert.Equal(t, "auto", u.Config["theme"], "GetMe 响应必须包含 config.theme")
}