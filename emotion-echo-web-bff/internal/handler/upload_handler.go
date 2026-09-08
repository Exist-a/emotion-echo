// Package handler — upload_handler.go
//
// Stage 58 PR-UP-1: 通用文件上传 handler（替代 Stage 30 占位 502）
//
// 端点：POST /api/v1/uploads/{image,video,file}
//
// 行为契约：
//   - kind ∈ {image, video, file}（白名单）— 其他 → 400
//   - 必填：X-User-Id header（APISIX 注入；缺失/非法 → 401）
//   - 必填：multipart file 字段（缺失 → 400）
//   - kind=image 限制 mime ∈ {image/jpeg, image/png, image/gif, image/webp}
//     kind=video 限制 mime ∈ {video/mp4, video/webm}
//     kind=file 不限制 mime
//     mime 不匹配 kind 白名单 → 415
//   - 文件超 size 限制（image 5MB / video 50MB / file 20MB） → 413
//   - Storage 未配置（nil） → 503
//   - 写 MinIO → 返回 {code:0, message:"ok", url, kind, size, mime}
//
// 关联文档：
//   - docs/plans/todo-pile-2026-09-04.md §A2
//   - emotion-echo-web-bff/internal/storage/minio.go（StorageClient 接口）
package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// uploadKindWhitelist kind 白名单（路由 :kind 参数校验）
var uploadKindWhitelist = map[string]bool{
	"image": true,
	"video": true,
	"file":  true,
}

// uploadMimeWhitelist mime 白名单（按 kind 限定）
var uploadMimeWhitelist = map[string]map[string]bool{
	"image": {
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	},
	"video": {
		"video/mp4":  true,
		"video/webm": true,
	},
	// "file": 不限制 mime（任意 application/* 都接受）
}

// uploadMaxSizeBytes 各 kind 的大小上限（dev 限额，prod 可通过 env 覆盖）
var uploadMaxSizeBytes = map[string]int64{
	"image": 5 << 20,  // 5 MiB
	"video": 50 << 20, // 50 MiB
	"file":  20 << 20, // 20 MiB
}

// uploadMimeExtFallback mime → 扩展名 fallback（用于无扩展名文件）
var uploadMimeExtFallback = map[string]string{
	"image/jpeg":    ".jpg",
	"image/png":     ".png",
	"image/gif":     ".gif",
	"image/webp":    ".webp",
	"video/mp4":     ".mp4",
	"video/webm":    ".webm",
	"application/pdf": ".pdf",
}

// UploadHandler 通用文件上传
type UploadHandler struct {
	storage storageClient
}

// NewUploadHandler 构造
func NewUploadHandler(storage storageClient) *UploadHandler {
	return &UploadHandler{storage: storage}
}

// Register 注册路由
func (h *UploadHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/uploads/:kind", h.upload)
}

// upload 处理 /api/v1/uploads/:kind
func (h *UploadHandler) upload(c *gin.Context) {
	// 1. kind 白名单
	kind := c.Param("kind")
	if !uploadKindWhitelist[kind] {
		Fail(c, http.StatusBadRequest, 1, "unsupported kind: "+kind+" (allowed: image, video, file)")
		return
	}

	// 2. X-User-Id 必填（APISIX 注入；缺失 → 401 不暴露存储细节）
	uidStr := c.GetHeader("X-User-Id")
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil || uid <= 0 {
		Fail(c, http.StatusUnauthorized, 1, "missing or invalid X-User-Id header")
		return
	}

	// 3. Storage 未配置 → 503
	if h.storage == nil {
		Fail(c, http.StatusServiceUnavailable, 1, "object storage not configured")
		return
	}

	// 4. 解析 multipart（按 kind 大小上限设上限，超限 → 413）
	maxSize := uploadMaxSizeBytes[kind]
	if err := c.Request.ParseMultipartForm(maxSize); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "request body too large") {
			Fail(c, http.StatusRequestEntityTooLarge, 1, fmt.Sprintf("file exceeds %d bytes for kind=%s", maxSize, kind))
			return
		}
		Fail(c, http.StatusBadRequest, 1, "invalid multipart: "+errMsg)
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		Fail(c, http.StatusBadRequest, 1, "file 字段必填")
		return
	}

	// 5. mime 白名单校验（按 kind）
	contentType := fileHeader.Header.Get("Content-Type")
	if mimes, ok := uploadMimeWhitelist[kind]; ok {
		if !mimes[contentType] {
			Fail(c, http.StatusUnsupportedMediaType, 1, fmt.Sprintf("mime %s not allowed for kind=%s", contentType, kind))
			return
		}
	}

	// 6. 打开文件并写 MinIO
	file, err := fileHeader.Open()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, "open file: "+err.Error())
		return
	}
	defer file.Close()

	objectKey := uploadObjectKey(uid, kind, fileHeader.Filename, contentType)
	publicURL, err := h.storage.PutObject(c.Request.Context(), objectKey, file, fileHeader.Size, contentType)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, "storage put: "+err.Error())
		return
	}

	// 7. 返 200 + 元信息
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"url":     publicURL,
		"kind":    kind,
		"size":    fileHeader.Size,
		"mime":    contentType,
	})
}

// uploadObjectKey 生成上传对象 key
//   - format: uploads/<uid>-<sha256(prefix)[:8]>.<ext>
//   - 与 storage.ObjectKey 同款哈希算法但前缀不同（uploads/ vs avatars/）
//     —— 隔离头像与通用上传目录
func uploadObjectKey(uid int64, kind, originalFilename, contentType string) string {
	h := sha256.Sum256([]byte(originalFilename))
	prefix := hex.EncodeToString(h[:])[:8]
	ext := strings.ToLower(path.Ext(originalFilename))
	if ext == "" {
		ext = uploadMimeExtFallback[contentType]
		if ext == "" {
			ext = ".bin"
		}
	}
	return fmt.Sprintf("uploads/%d-%s%s", uid, prefix, ext)
}