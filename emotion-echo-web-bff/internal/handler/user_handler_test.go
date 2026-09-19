// Package handler — user_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.35 RED: user handler 契约测试
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeUserClient 实现 downstream.UserClient
type fakeUserClient struct {
	me      *downstream.UserInfo
	updated *downstream.UserInfo
	byID    *downstream.UserInfo
	login   *downstream.UserInfo
	reg     *downstream.UserInfo
	err     error
	loginErr error
	regErr  error
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
func (f *fakeUserClient) Login(_ context.Context, _, _ string) (*downstream.UserInfo, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return f.login, nil
}
func (f *fakeUserClient) Register(_ context.Context, _, _, _ string, _ []downstream.SecurityQuestion) (*downstream.UserInfo, error) {
	if f.regErr != nil {
		return nil, f.regErr
	}
	return f.reg, nil
}

// Sprint 1 PR-4c-3: fake ResetPassword（仅用于 auth_handler_test 编译）
func (f *fakeUserClient) ResetPassword(_ context.Context, _ downstream.ResetPasswordReq) (*downstream.UserInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.me, nil
}

// E2E-06: fake VerifySecurityAnswer
func (f *fakeUserClient) VerifySecurityAnswer(_ context.Context, _ int64, _ int, _ string) error {
	return f.err
}

// R-01 #1: fake VerifySecurityAnswerByUsername
func (f *fakeUserClient) VerifySecurityAnswerByUsername(_ context.Context, _ string, _ int, _ string) error {
	return f.err
}

// E2E-07: fake GetSecurityQuestionsByUsername
func (f *fakeUserClient) GetSecurityQuestionsByUsername(_ context.Context, _ string) ([]downstream.SecurityQuestionInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
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
	decodeData(t, w.Body.Bytes(), &body)
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

// E2E-11: toProfileVM 必须映射 AvatarURL（不硬编码 ""）
func TestToProfileVM_MapsAvatarURL(t *testing.T) {
	avatar := "https://minio.example.com/avatars/1-abc.jpg"
	u := &downstream.UserInfo{
		UserID:     42,
		Account:    "alice",
		Nickname:   "Alice",
		AvatarURL:  avatar,
		CreatedAt:  1700000000,
	}
	vm := toProfileVM(u)
	assert.Equal(t, "42", vm.ID)
	assert.Equal(t, "alice", vm.Username)
	assert.Equal(t, "Alice", vm.Nickname)
	assert.Equal(t, avatar, vm.Avatar, "Avatar 应从 AvatarURL 映射")
	assert.NotEmpty(t, vm.CreatedAt, "CreatedAt 应有值")
}

// E2E-11: toProfileVM AvatarURL 为空时 Avatar 也应为空字符串
func TestToProfileVM_EmptyAvatar(t *testing.T) {
	u := &downstream.UserInfo{UserID: 1, Account: "bob", Nickname: "Bob"}
	vm := toProfileVM(u)
	assert.Equal(t, "", vm.Avatar)
}

// E2E-11: profile 端点应调 user-svc 返回真实数据（非硬编码 mock）
func TestUserHandler_Profile_ReturnsRealData(t *testing.T) {
	avatar := "https://minio.example.com/avatars/7-test.jpg"
	fc := &fakeUserClient{me: &downstream.UserInfo{
		UserID:    7,
		Account:   "alice",
		Nickname:  "Alice",
		AvatarURL: avatar,
		CreatedAt: 1700000000,
	}}
	r := newUserRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	// 注入 CtxUserIDKey（模拟 APISIX jwt-auth 中间件行为）
	ctx := context.WithValue(req.Context(), sharedmw.CtxUserIDKey{}, int64(7))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body ProfileVM
	decodeData(t, w.Body.Bytes(), &body)
	assert.Equal(t, "7", body.ID)
	assert.Equal(t, "Alice", body.Nickname, "昵称应来自 user-svc，非硬编码")
	assert.Equal(t, avatar, body.Avatar, "头像应来自 user-svc，非空字符串")
	assert.Equal(t, "alice", body.Username)
}

// E2E-11: profile 端点未认证时应返回 401
func TestUserHandler_Profile_NoAuth_Returns401(t *testing.T) {
	r := newUserRouter(&fakeUserClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	// 不注入 CtxUserIDKey
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
