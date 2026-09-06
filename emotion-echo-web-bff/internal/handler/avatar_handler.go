// Package handler — avatar_handler.go
//
// Sprint 1 PR-4c-2: 用户头像上传（multipart → MinIO → user-svc UpdateMe）
//
// 行为契约：
//   - POST /api/v1/user/avatar 接 multipart (avatar file)
//   - 必填：avatar file 字段 + X-User-Id header（APISIX 注入）
//   - 文件名命名约定 storage.ObjectKey(uid, originalFilename)
//   - 写 MinIO + 调 user-svc UpdateMe(AvatarURL)
//   - 返 {avatar: <public URL>}
//   - 缺 X-User-Id → 401（不暴露存储细节给未授权请求）
//   - 缺 file → 400
//   - Storage 未配置 → 503（PR-4b MinIO 装配失败时的兜底）
//   - user-svc 写库失败 → 500
//
// Sprint 1 plan §PR-4c-2
package handler

import (
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/storage"

	"github.com/gin-gonic/gin"
)

// storageClient 内部接口（避免 import cycle；等价于 storage.StorageClient）
// 这里重复声明是因为 handler 包不应直接 import storage（已通过 ServiceContext 注入）
type storageClient = storage.StorageClient

// AvatarHandler 用户头像上传
type AvatarHandler struct {
	user    downstream.UserClient
	storage storageClient
}

// NewAvatarHandler 构造
func NewAvatarHandler(user downstream.UserClient, storage storageClient) *AvatarHandler {
	return &AvatarHandler{user: user, storage: storage}
}

// Register 注册路由：POST /api/v1/user/avatar
func (h *AvatarHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/user/avatar", h.upload)
}

// upload 处理头像上传
func (h *AvatarHandler) upload(c *gin.Context) {
	// 1. 校验 X-User-Id header（APISIX 注入；缺失/非法 → 401）
	uidStr := c.GetHeader("X-User-Id")
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil || uid <= 0 {
		Fail(c, http.StatusUnauthorized, 1, "missing or invalid X-User-Id header")
		return
	}

	// 2. Storage 未配置 → 503
	if h.storage == nil {
		Fail(c, http.StatusServiceUnavailable, 1, "object storage not configured")
		return
	}

	// 3. 解析 multipart（上限 2MB，前端 beforeAvatarUpload 已校验）
	if err := c.Request.ParseMultipartForm(2 << 20); err != nil {
		Fail(c, http.StatusBadRequest, 1, "invalid multipart: "+err.Error())
		return
	}
	fileHeader, err := c.FormFile("avatar")
	if err != nil {
		Fail(c, http.StatusBadRequest, 1, "avatar 字段必填")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, "open file: "+err.Error())
		return
	}
	defer file.Close()

	// 4. 写 MinIO
	objectKey := storage.ObjectKey(uid, fileHeader.Filename)
	publicURL, err := h.storage.PutObject(c.Request.Context(), objectKey, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, "storage put: "+err.Error())
		return
	}

	// 5. 同步 user-svc 写库（UpdateMe AvatarURL）
	urlStr := publicURL
	_, err = h.user.UpdateMe(c.Request.Context(), downstream.UpdateProfileReq{
		AvatarURL: &urlStr,
	})
	if err != nil {
		// 已写 MinIO 但写库失败 — 此处不删 MinIO（防删了用户没换上的图）
		// 留运维通过 MinIO 控制台清理；前端感知 500 重试
		Fail(c, http.StatusInternalServerError, 1, "user-svc update: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"avatar":  publicURL,
	})
}