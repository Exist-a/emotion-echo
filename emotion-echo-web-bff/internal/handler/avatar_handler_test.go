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
	gotCtx context.Context
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
	f.gotCtx = ctx
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
func (f *fakeAvatarUserClient) Register(ctx context.Context, username, password, code string, questions []downstream.SecurityQuestion) (*downstream.UserInfo, error) {
	return nil, nil
}

// Sprint 1 PR-4c-3: fake ResetPassword
func (f *fakeAvatarUserClient) ResetPassword(ctx context.Context, req downstream.ResetPasswordReq) (*downstream.UserInfo, error) {
	return nil, nil
}

// E2E-06: fake VerifySecurityAnswer
func (f *fakeAvatarUserClient) VerifySecurityAnswer(ctx context.Context, userID int64, questionOrder int, answer string) error {
	return nil
}

// R-01 #1: fake VerifySecurityAnswerByUsername
func (f *fakeAvatarUserClient) VerifySecurityAnswerByUsername(ctx context.Context, username string, questionOrder int, answer string) error {
	return nil
}

// E2E-07: fake GetSecurityQuestionsByUsername
func (f *fakeAvatarUserClient) GetSecurityQuestionsByUsername(ctx context.Context, username string) ([]downstream.SecurityQuestionInfo, error) {
	return nil, nil
}

// fakeStorage 模拟 StorageClient
type fakeStorage struct {
	putURL string
	err    error
	// E2E-27 #4：捕获 PutObject 收到的 ctx，用于断言 deadline 存在（防停机挂起）
	putCtx context.Context
	// E2E-27 M1：GET image 端点用例的可配置对象内容与错误
	getObjBytes []byte
	getObjCT    string
	getObjErr   error
	gotGetObjKey string
}

func (f *fakeStorage) PutObject(ctx context.Context, key string, r io.Reader, size int64, ct string) (string, error) {
	f.putCtx = ctx
	if f.err != nil {
		return "", f.err
	}
	return f.putURL, nil
}
func (f *fakeStorage) GetObjectURL(key string) string { return "" }
func (f *fakeStorage) GetObject(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	// M1 起 avatar GET image 端点会调用 GetObject（原「不应调用」防护随端点新增解除）
	f.gotGetObjKey = key
	if f.getObjErr != nil {
		return nil, "", 0, f.getObjErr
	}
	return io.NopCloser(bytes.NewReader(f.getObjBytes)), f.getObjCT, int64(len(f.getObjBytes)), nil
}
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
	// E2E-27 M1 / F-116：响应与落库一律网关相对路径（ADR-2026-09 决策 1），
	// 不得下发 PublicBaseURL 绝对地址（宿主浏览器可用纯属端口转发巧合）。
	assert.Equal(t, "/api/v1/user/avatar/image/7-2cdae8ed.jpg", *user.gotReq.AvatarURL,
		"落库必须是网关相对路径（sha8(me.jpg)=2cdae8ed）")

	var got struct {
		Data struct {
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	// E2E-11 复查：原断言读顶层 got["avatar"]，把"无 data 包装"的 bug 固化了。
	// 现按前端真实消费路径（useApi 返回 data.data）断言 data.avatar。
	assert.Equal(t, "/api/v1/user/avatar/image/7-2cdae8ed.jpg", got.Data.Avatar,
		"响应必须是网关相对路径（E2E-27 M1/F-116）")
}

// E2E-27 M1：GET /api/v1/user/avatar/image/:filekey 反代端点（ADR-2026-09 决策 1/
// 3 的 avatar 落地——voice 反代同型）。相对 URL 的消费方（浏览器 <img> 经网关
// getFullUrl 解析）都指向这个端点；端点本身须具备 voice 同款防御语义。
func TestAvatarHandler_ImageGet_Success(t *testing.T) {
	sto := &fakeStorage{getObjBytes: []byte("PNG-BYTES"), getObjCT: "image/png"}
	r := newAvatarRouter(&fakeAvatarUserClient{}, sto)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/avatar/image/7-2cdae8ed.jpg", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"), "必须回读真实 Content-Type")
	assert.Equal(t, []byte("PNG-BYTES"), w.Body.Bytes(), "body 必须等于 storage 流")
	assert.Equal(t, "avatars/7-2cdae8ed.jpg", sto.gotGetObjKey, "handler 补 avatars/ 前缀")
}

func TestAvatarHandler_ImageGet_ObjectNotFound_Returns404(t *testing.T) {
	sto := &fakeStorage{getObjErr: errors.New("StatObject(avatars/x.jpg): The specified key does not exist.")}
	r := newAvatarRouter(&fakeAvatarUserClient{}, sto)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/avatar/image/missing.jpg", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"缺失对象必须 404（ADR 决策 3；minio-go 真实文案），body: %s", w.Body.String())
	// 断言 body 是 handler 的 JSON——gin 路由未注册时也返 404（空 body），
	// 仅断言状态码会让「端点不存在」假通过。
	assert.Contains(t, w.Body.String(), `"code"`, "必须是 handler 的 404 JSON 响应而非路由级空 404")
}

func TestAvatarHandler_ImageGet_PathTraversal_Returns400(t *testing.T) {
	sto := &fakeStorage{getObjBytes: []byte("x")}
	r := newAvatarRouter(&fakeAvatarUserClient{}, sto)

	for _, key := range []string{"..vhidden", "abc..xyz"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/avatar/image/"+key, nil)
		req.Header.Set("X-User-Id", "7")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "filekey 含 .. 必须 400（key=%s）", key)
	}
	assert.Empty(t, sto.gotGetObjKey, "被拦截的非法 key 不应触达 storage")
}

func TestAvatarHandler_ImageGet_NilStorage_Returns503(t *testing.T) {
	r := newAvatarRouter(&fakeAvatarUserClient{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/avatar/image/7-x.jpg", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"storage 未配置必须 503（与上传语义一致），body: %s", w.Body.String())
}

// E2E-27 #4 运行时实证（2026-10-03）：停 MinIO 容器后 avatar 上传挂起——
// curl --max-time 15 得 000（0 字节），既非 500 也非 503。根因：
// PutObject 用裸 request ctx（无 deadline）+ minio-go 内部重试 ⇒ 请求被拖死。
// 通过标准（plan #4）：存储停机必须**快速失败**返 503（与 handler 契约
// "Storage 未配置 → 503" 同属存储不可用语义），且 ctx 必须带 deadline。
func TestAvatarHandler_Upload_StorageDown_FastFail503(t *testing.T) {
	sto := &fakeStorage{err: errors.New("dial tcp 127.0.0.1:9000: connect: connection refused")}
	r := newAvatarRouter(&fakeAvatarUserClient{}, sto)

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

	require.NotNil(t, sto.putCtx, "PutObject 必须被调用")
	_, hasDeadline := sto.putCtx.Deadline()
	assert.True(t, hasDeadline,
		"PutObject ctx 必须带 deadline——运行时实证无 deadline 时停机挂起 >15s")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"存储连接类失败必须 503 快速失败（存储不可用语义），body: %s", w.Body.String())
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

// TestAvatarHandler_PassesUserIDToDownstream 契约（E2E-11）：
// 调 user-svc UpdateMe 的 ctx 必须携带 user_id（经 session.WithRequestAuth 注入），
// 否则 gRPC 客户端的 withUserID(ctx) 无值 → 不带 x-user-id metadata →
// user-svc 拦截器拒绝。
//
// dev 实测（2026-09-19）BFF 日志：
//
//	[grpc-client] method=/emotion_user.v1.UserService/UpdateProfile
//	  latency=0ms err=rpc error: code = Unauthenticated
//	  desc = missing x-user-id metadata
//
// 前端表现为头像上传 500（MinIO 对象已写入，但 user-svc 落库失败 ⇒ 孤儿对象 + 头像不生效）。
func TestAvatarHandler_PassesUserIDToDownstream(t *testing.T) {
	fc := &fakeAvatarUserClient{}
	r := newAvatarRouter(fc, &fakeStorage{putURL: "http://minio/avatars/7-x.jpg"})

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

	require.Equal(t, http.StatusOK, w.Code, "上传应成功")
	require.NotNil(t, fc.gotCtx, "UpdateMe 应被调用")

	uid, ok := downstream.UserIDFromContext(fc.gotCtx)
	assert.True(t, ok, "E2E-11: 传给 UpdateMe 的 ctx 必须能取到 user_id")
	assert.Equal(t, int64(7), uid,
		"E2E-11: 传给 UpdateMe 的 ctx 必须带 user_id（用 session.WithRequestAuth 包装）。"+
			"原实现传 c.Request.Context() ⇒ gRPC 无 x-user-id metadata ⇒ "+
			"user-svc 返 Unauthenticated ⇒ 头像上传 500。")
}
// TestAvatarHandler_OversizedFile_Rejected 契约（E2E-11 复查）：
// 服务端必须自己拦 >2MB 的文件，不能只依赖前端 beforeAvatarUpload。
//
// 原实现只调 `ParseMultipartForm(2 << 20)` —— 该参数是「内存/磁盘分界」，
// **不是上限**（Go 实际容忍 maxMemory + 10MB 的请求体），所以绕过前端直传
// 3MB/10MB 文件都会被接受并写入 MinIO。
func TestAvatarHandler_OversizedFile_Rejected(t *testing.T) {
	fc := &fakeAvatarUserClient{}
	fs := &fakeStorage{putURL: "http://minio/should-not-be-called"}
	r := newAvatarRouter(fc, fs)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("avatar", "huge.png")
	_, _ = io.CopyN(fw, bytes.NewReader(make([]byte, 3<<20)), 3<<20) // 3MB
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/avatar", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"E2E-11: 超过 2MB 的头像必须被服务端拒绝（413）。"+
			"原实现只有前端拦截，绕过前端即可写入任意大小文件到 MinIO。")
	assert.Nil(t, fc.gotReq, "被拒绝的请求不应调 user-svc")
}

// TestAvatarHandler_ResponseUsesDataWrapper 契约（E2E-11 复查，IAB 实测发现）：
// 响应必须是与其他所有端点一致的 {code, message, data} 包装，头像 URL 放在 data.avatar。
//
// 前端 `useApi.request()` 统一 `return data.data`，而本端点原实现把 avatar 放在**顶层**：
//   {"avatar":"http://...","code":0,"message":"ok"}   ← 无 data 字段
// ⇒ `post<{avatar:string}>()` 返回 undefined ⇒ `res.avatar` 抛 TypeError
//   被 catch 捕获 ⇒ 用户看到"头像上传失败，请重试"的**错误提示**，
//   但服务端其实已经写成功（DB 已更新）—— 典型"提示与实际相反"。
//
// 实测证据（2026-09-19 IAB，浏览器内 fetch 直读响应体）：
//   {"avatar":"http://localhost:9000/avatars/avatars/1-60a9c298.png","code":0,"message":"ok"}
// 同时前端预览永远停在本地 blob URL（`form.value.avatarPath` 未被服务端 URL 覆盖）。
func TestAvatarHandler_ResponseUsesDataWrapper(t *testing.T) {
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

	require.Equal(t, http.StatusOK, w.Code)

	var wrapped struct {
		Code int `json:"code"`
		Data struct {
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &wrapped))
	assert.Equal(t, 0, wrapped.Code)
	assert.Equal(t, "http://localhost:9000/avatars/avatars/7-abc.jpg", wrapped.Data.Avatar,
		"E2E-11: 头像 URL 必须在 data.avatar 内（与 BFF 其他端点一致）。"+
			"原实现在顶层返回 avatar ⇒ useApi 的 data.data 为 undefined ⇒ "+
			"前端 res.avatar 抛错 ⇒ 上传成功却提示失败。")
}
