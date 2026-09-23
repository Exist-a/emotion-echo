// Package handler — upload_handler_size_test.go
//
// E2E-F-123：原 uploadMaxSizeBytes 限制通过 ParseMultipartForm(maxSize) 设置，但
// Go 的 ParseMultipartForm maxSize 是内存缓存阈值（非请求体上限），超大文件
// 仍能通过。修法：FormFile 后显式 fileHeader.Size > maxSize 校验。
//
// 本测试钉：image > 5MB → 413；file > 20MB → 413；恰好等于上限 → 200（边界包含）。
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMultipart 构造 multipart 请求（含指定字节数的 file 字段）
func buildMultipart(t *testing.T, fieldName, filename, contentType string, sizeBytes int64) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	// 写 sizeBytes 个 'x'（不模拟真实文件格式；mime 不在 image 白名单时也走 size check）
	_, err = part.Write(bytes.Repeat([]byte{'x'}, int(sizeBytes)))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return body, w.FormDataContentType()
}

func TestUploadHandler_FileExceedsLimit_Returns413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	sto := &fakeUploadStorage{}
	(&UploadHandler{storage: sto}).Register(r)

	// file kind 限额 20 MB（20 << 20），构造 21 MB
	body, ct := buildMultipart(t, "file", "huge.txt", "text/plain", 21<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/file", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "42")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"21MB > 20MB file limit 必须返 413；body=%s", w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "exceeds kind=file limit", "错误消息应含上限说明")
}

func TestUploadHandler_ImageExceedsLimit_Returns413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	sto := &fakeUploadStorage{}
	(&UploadHandler{storage: sto}).Register(r)

	// image kind 限额 5 MB，构造 6 MB image/jpeg
	body, ct := buildMultipart(t, "file", "big.jpg", "image/jpeg", 6<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "42")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"6MB > 5MB image limit 必须返 413；body=%s", w.Body.String())
}

func TestUploadHandler_AtLimit_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	sto := &fakeUploadStorage{}
	(&UploadHandler{storage: sto}).Register(r)

	// 恰好 20 MB（边界包含）→ 应 200
	body, ct := buildMultipart(t, "file", "at-limit.txt", "text/plain", 20<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/file", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User-Id", "42")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"恰好 20MB（等于上限）应通过；body=%s", w.Body.String())
}

// silence unused import in some Go versions
var _ = fmt.Sprintf
var _ = strings.Contains
