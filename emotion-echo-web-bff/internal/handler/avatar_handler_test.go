// Package handler — avatar_handler_test.go
//
// Sprint 1 PR-4c-2: avatar_handler 单元测试
//
// 行为契约：
//   - POST /api/v1/user/avatar 接 multipart (avatar file, X-User-Id 必填)
//   - 调 storage.PutObject (MinIO) + user-svc UpdateMe(AvatarURL)
//   - 返回 {avatar: <public URL>}
//   - 缺 X-User-Id → 401
//   - 缺 file → 400
//   - Storage 未配置 (nil) → 503
//   - user-svc 写库失败 → 500

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAvatarUserClient 模拟 UserClient（仅 avatar handler 用到的最小实现）
type fakeAvatarUserClient struct {
	gotReq *downstream.UpdateProfileReq
	err    error
}

func (f *fakeAvatarUserClient) GetMe(ctx context.Context) (*downstream.UserInfo, error) {
	return nil, nil
}
func (f *fakeAvatarUserClient) GetByID(ctx context.Context, id int64) (*downstream.UserInfo, error) {
	return nil, nil
}
func (f *fakeAvatarUserClient) UpdateMe(ctx context.Context, req downstream.UpdateProfileReq) (*downstream.UserInfo, error) {
	f.gotReq = &req
	if f.err != nil {
		return nil, f.err
	}
	return &downstream.UserInfo{UserID: 7}, nil
}
func (f *fakeAvatarUserClient) UsernameExists(ctx context.Context, username string) (bool, error) {
	return false, nil
}
func (f *fakeAvatarUserClient) Login(ctx context.Context, username, password string) (*downstream.UserInfo, error) {
	return nil, nil
}
func (f *fakeAvatarUserClient) Register(ctx context.Context, username, password, code string) (*downstream.UserInfo, error) {
	return nil, nil
}

// Sprint 1 PR-4c-3: fake ResetPassword
func (f *fakeAvatarUserClient) ResetPassword(ctx context.Context, req downstream.ResetPasswordReq) (*downstream.UserInfo, error) {
	return nil, nil
}

// fakeStorage 模拟 StorageClient
type fakeStorage struct {
	putURL string
	err    error
}

func (f *fakeStorage) PutObject(ctx context.Context, key string, r io.Reader, size int64, ct string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.putURL, nil
}
func (f *fakeStorage) GetObjectURL(key string) string                    { return "" }
func (f *fakeStorage) RemoveObject(ctx context.Context, key string) error { return nil }
func (f *fakeStorage) HealthCheck(ctx context.Context) error             { return nil }

func newAvatarRouter(user downstream.UserClient, storage storageClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&AvatarHandler{user: user, storage: storage}).Register(r)
	return r
}

func TestAvatarHandler_Upload_Success(t *testing.T) {
	user := &fakeAvatarUserClient{}
	sto := &fakeStorage{putURL: "http://localhost:9000/avatars/avatars/7-abc.jpg"}
	r := newAvatarRouter(user, sto)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("avatar", "me.jpg")
	_, _ = io.WriteString(fw, "fake jpg bytes")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.NotNil(t, user.gotReq, "应调用 user-svc UpdateMe")
	require.NotNil(t, user.gotReq.AvatarURL)
	assert.Equal(t, "http://localhost:9000/avatars/avatars/7-abc.jpg", *user.gotReq.AvatarURL)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "http://localhost:9000/avatars/avatars/7-abc.jpg", got["avatar"])
}

func TestAvatarHandler_MissingXUserId_Returns401(t *testing.T) {
	r := newAvatarRouter(&fakeAvatarUserClient{}, &fakeStorage{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("avatar", "me.jpg")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// 故意缺 X-User-Id
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAvatarHandler_InvalidXUserId_Returns401(t *testing.T) {
	r := newAvatarRouter(&fakeAvatarUserClient{}, &fakeStorage{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("avatar", "me.jpg")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "not-a-number")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAvatarHandler_MissingFile_Returns400(t *testing.T) {
	r := newAvatarRouter(&fakeAvatarUserClient{}, &fakeStorage{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	// 故意缺 avatar 字段
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAvatarHandler_StorageNil_Returns503(t *testing.T) {
	r := newAvatarRouter(&fakeAvatarUserClient{}, nil) // storage=nil

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("avatar", "me.jpg")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAvatarHandler_UserSvcError_Returns500(t *testing.T) {
	user := &fakeAvatarUserClient{err: errors.New("db connection lost")}
	sto := &fakeStorage{putURL: "http://x/avatars/7-abc.jpg"}
	r := newAvatarRouter(user, sto)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("avatar", "me.jpg")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAvatarHandler_Register_PathContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&AvatarHandler{user: &fakeAvatarUserClient{}, storage: &fakeStorage{}}).Register(r)
	assert.Equal(t, 1, len(r.Routes()))
	for _, ri := range r.Routes() {
		assert.Equal(t, "/api/v1/user/avatar", ri.Path)
		assert.Equal(t, http.MethodPost, ri.Method)
	}
}