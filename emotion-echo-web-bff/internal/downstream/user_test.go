// Package downstream — user_test.go
//
// Stage 30 / stage-30-web-bff.md T2.12-14 RED: UserClient 契约测试
package downstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserClient_GetMe_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/users/me", r.URL.Path)
		assert.Equal(t, "Bearer jwt-1", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(userWrapper{User: &UserInfo{
			UserID: 7, Account: "alice", Phone: "13800000000", Nickname: "Alice",
		}})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	u, err := c.GetMe(WithJWT(context.Background(), "jwt-1"))
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, int64(7), u.UserID)
	assert.Equal(t, "Alice", u.Nickname)
}

func TestUserClient_UpdateMe_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v1/users/me", r.URL.Path)

		var req UpdateProfileReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.NotNil(t, req.Nickname)
		assert.Equal(t, "NewNick", *req.Nickname)

		_ = json.NewEncoder(w).Encode(userWrapper{User: &UserInfo{UserID: 7, Nickname: "NewNick"}})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	nick := "NewNick"
	u, err := c.UpdateMe(WithJWT(context.Background(), "jwt-1"), UpdateProfileReq{Nickname: &nick})
	require.NoError(t, err)
	assert.Equal(t, "NewNick", u.Nickname)
}

func TestUserClient_GetByID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/users/42", r.URL.Path)
		_ = json.NewEncoder(w).Encode(userWrapper{User: &UserInfo{UserID: 42, Account: "bob"}})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	u, err := c.GetByID(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, int64(42), u.UserID)
	assert.Equal(t, "bob", u.Account)
}

func TestUserClient_Unauthorized_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: invalid or missing JWT"})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.GetMe(context.Background()) // 无 JWT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or missing JWT")
}

func TestUserClient_MissingUserField_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`)) // 缺 user 字段
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.GetMe(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'user'")
}

func TestUserClient_GetByID_PathEncoding(t *testing.T) {
	// 验证 id 用 strconv 编码（不是 %d 占位符注入）
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(userWrapper{User: &UserInfo{UserID: 99}})
	}))
	defer srv.Close()

	c := NewUserClient(UserClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, _ = c.GetByID(context.Background(), 99)
	assert.Equal(t, "/api/v1/users/99", gotPath, "path 应为 /api/v1/users/" + strconv.FormatInt(99, 10))
}
