// Package handler — upload_handler_test.go
//
// Stage 58 PR-UP-1: 通用上传 handler 单元测试（A2 文件上传后端实现）
//
// 行为契约：
//   - POST /api/v1/uploads/:kind 接 multipart (file 字段)
//   - kind ∈ {image, video, file} — 其他 kind → 400
//   - 必填：X-User-Id header（APISIX 注入）+ multipart file 字段
//   - kind=image 限制 mime ∈ {image/jpeg, image/png, image/gif, image/webp}
//   - kind=video 限制 mime ∈ {video/mp4, video/webm}
//   - kind=file 不限制 mime
//   - 写 MinIO → 返回 {url, kind, size, mime}
//   - 缺 X-User-Id → 401
//   - 缺 file → 400
//   - Storage 未配置 (nil) → 503
//   - mime 不匹配 kind 白名单 → 415
//   - 文件超 size 限制（image 5MB / video 50MB / file 20MB） → 413
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUploadStorage 通用上传测试专用 storage（avatar_test 也有 fakeStorage 但我们不复用）
type fakeUploadStorage struct {
	putURL string
	err    error
	gotKey string
	gotSize int64
	gotCT   string
}

func (f *fakeUploadStorage) PutObject(ctx context.Context, key string, r io.Reader, size int64, ct string) (string, error) {
	f.gotKey = key
	f.gotSize = size
	f.gotCT = ct
	if f.err != nil {
		return "", f.err
	}
	return f.putURL, nil
}
func (f *fakeUploadStorage) GetObjectURL(key string) string                    { return "" }
func (f *fakeUploadStorage) RemoveObject(ctx context.Context, key string) error { return nil }
func (f *fakeUploadStorage) HealthCheck(ctx context.Context) error             { return nil }

// 复用 storage.StorageClient 类型别名（handler 包已有 storageClient 别名）
var _ storage.StorageClient = (*fakeUploadStorage)(nil)

func newUploadRouter(sto storageClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&UploadHandler{storage: sto}).Register(r)
	return r
}

// multipartBuild 构造带 file 字段的 multipart body
// 使用 CreateFormFile 简化版（multipart 标准库默认 Content-Type 为 application/octet-stream，
// 但 fileHeader 在 gin 上可通过 form-data 自定义）
func multipartBuild(t *testing.T, filename, contentType, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	// 用 CreatePart 显式设 Content-Type（CreateFormFile 默认是 application/octet-stream）
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", contentType)
	fw, err := mw.CreatePart(h)
	require.NoError(t, err)
	_, err = io.WriteString(fw, content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return body, mw.FormDataContentType()
}

// ============ 成功路径 ============

func TestUploadHandler_Image_Success(t *testing.T) {
	sto := &fakeUploadStorage{putURL: "http://localhost:9000/avatars/uploads/7-abc.jpg"}
	r := newUploadRouter(sto)

	body, ct := multipartBuild(t, "photo.jpg", "image/jpeg", "fake jpg bytes")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "image", got["kind"])
	assert.Equal(t, "http://localhost:9000/avatars/uploads/7-abc.jpg", got["url"])
	assert.Equal(t, "image/jpeg", got["mime"])
	// multipart part 边界可能让 fileHeader.Size 比原始字符串多 1-2 字节（CRLF）
	// 用 >= 而非精确等号
	sizeVal, ok := got["size"].(float64)
	require.True(t, ok, "size 字段应为数字")
	assert.GreaterOrEqual(t, int64(sizeVal), int64(13), "size 应至少含 'fake jpg bytes'(13) 字节")

	// 验证 storage 收到正确参数
	assert.True(t, strings.HasPrefix(sto.gotKey, "uploads/7-"), "key 应以 uploads/<uid>- 开头: %s", sto.gotKey)
	assert.Equal(t, "image/jpeg", sto.gotCT)
}

func TestUploadHandler_Video_Success(t *testing.T) {
	sto := &fakeUploadStorage{putURL: "http://localhost:9000/avatars/uploads/3-xyz.mp4"}
	r := newUploadRouter(sto)

	body, ct := multipartBuild(t, "clip.mp4", "video/mp4", "fake mp4 data")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/video", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "3")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "video", got["kind"])
	assert.Equal(t, "video/mp4", got["mime"])
}

func TestUploadHandler_File_Success(t *testing.T) {
	sto := &fakeUploadStorage{putURL: "http://localhost:9000/avatars/uploads/5-qwe.pdf"}
	r := newUploadRouter(sto)

	// kind=file 不限制 mime，pdf OK
	body, ct := multipartBuild(t, "doc.pdf", "application/pdf", "fake pdf content")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/file", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "5")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "file", got["kind"])
	assert.Equal(t, "application/pdf", got["mime"])
}

// ============ 校验失败 ============

func TestUploadHandler_InvalidKind_Returns400(t *testing.T) {
	r := newUploadRouter(&fakeUploadStorage{})

	body, ct := multipartBuild(t, "x.bin", "application/octet-stream", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/audio", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadHandler_MissingXUserId_Returns401(t *testing.T) {
	r := newUploadRouter(&fakeUploadStorage{})

	body, ct := multipartBuild(t, "x.jpg", "image/jpeg", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", ct)
	// 故意缺 X-User-Id
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUploadHandler_MissingFile_Returns400(t *testing.T) {
	r := newUploadRouter(&fakeUploadStorage{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadHandler_StorageNil_Returns503(t *testing.T) {
	r := newUploadRouter(nil) // storage=nil

	body, ct := multipartBuild(t, "x.jpg", "image/jpeg", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestUploadHandler_ImageMimeMismatch_Returns415(t *testing.T) {
	r := newUploadRouter(&fakeUploadStorage{})

	// kind=image 但 mime 是 application/pdf → 应 415
	body, ct := multipartBuild(t, "doc.pdf", "application/pdf", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestUploadHandler_StorageError_Returns500(t *testing.T) {
	sto := &fakeUploadStorage{err: errors.New("S3 timeout")}
	r := newUploadRouter(sto)

	body, ct := multipartBuild(t, "x.jpg", "image/jpeg", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}