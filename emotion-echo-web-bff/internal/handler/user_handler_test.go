// Package handler — user_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.35 RED: user handler 契约测试
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUserClient 实现 downstream.UserClient
type fakeUserClient struct {
	me      *downstream.UserInfo
	updated *downstream.UserInfo
	byID    *downstream.UserInfo
	err     error
}

func (f *fakeUserClient) GetMe(_ context.Context) (*downstream.UserInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.me, nil
}
func (f *fakeUserClient) UpdateMe(_ context.Context, _ downstream.UpdateProfileReq) (*downstream.UserInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.updated, nil
}
func (f *fakeUserClient) GetByID(_ context.Context, _ int64) (*downstream.UserInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byID, nil
}

func newUserRouter(client downstream.UserClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&UserHandler{user: client}).Register(r)
	return r
}

func TestUserHandler_GetMe_Success(t *testing.T) {
	r := newUserRouter(&fakeUserClient{me: &downstream.UserInfo{UserID: 7, Account: "alice", Nickname: "Alice"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer jwt-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body struct {
		User downstream.UserInfo `json:"user"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, int64(7), body.User.UserID)
	assert.Equal(t, "Alice", body.User.Nickname)
}

func TestUserHandler_UpdateMe_Success(t *testing.T) {
	updated := &downstream.UserInfo{UserID: 7, Nickname: "NewNick"}
	r := newUserRouter(&fakeUserClient{updated: updated})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me",
		bytes.NewReader([]byte(`{"nickname":"NewNick"}`)))
	req.Header.Set("Authorization", "Bearer jwt-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"nickname":"NewNick"`)
}

func TestUserHandler_GetByID_Success(t *testing.T) {
	r := newUserRouter(&fakeUserClient{byID: &downstream.UserInfo{UserID: 42, Account: "bob"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"userId":42`)
}

func TestUserHandler_GetByID_InvalidID_Returns400(t *testing.T) {
	r := newUserRouter(&fakeUserClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid user id")
}

func TestUserHandler_UpstreamError_Returns502(t *testing.T) {
	r := newUserRouter(&fakeUserClient{err: &downstream.APIError{StatusCode: http.StatusInternalServerError, Msg: "db down"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "db down")
}
