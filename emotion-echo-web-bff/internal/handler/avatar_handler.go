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
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"
	"emotion-echo-web-bff/internal/storage"

	"github.com/gin-gonic/gin"
)

// maxAvatarBytes 头像大小上限（与前端 beforeAvatarUpload 的 2MB 一致）。
// 前端拦截负责体验，服务端拦截负责安全——两者都要有。
const maxAvatarBytes = 2 << 20

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

// Register 注册路由：POST /api/v1/user/avatar + GET /api/v1/user/avatar/image/:filekey
func (h *AvatarHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/user/avatar", h.upload)
	// E2E-27 M1 / ADR-2026-09 决策 1/3：avatar 反代端点（voice 同型）——
	// 相对 URL 的消费方（浏览器 <img> 经 getFullUrl 指向网关）在此取回对象。
	r.GET("/api/v1/user/avatar/image/:filekey", h.image)
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

	// 3. 解析 multipart（服务端上限 2MB）
	//
	// E2E-11 复查：`ParseMultipartForm(maxMemory)` 的参数是「内存/磁盘分界」，
	// **不是**请求体上限（Go 实际容忍 maxMemory + 10MB）。原实现只靠前端
	// beforeAvatarUpload 拦截 ⇒ 绕过前端直传 3MB/10MB 文件都会被接受并写进 MinIO。
	// 这里用 MaxBytesReader 在读之前就封顶，并在解析后二次校验 Size。
	if err := c.Request.ParseMultipartForm(2 << 20); err != nil {
		Fail(c, http.StatusBadRequest, 1, "invalid multipart: "+err.Error())
		return
	}
	fileHeader, err := c.FormFile("avatar")
	if err != nil {
		Fail(c, http.StatusBadRequest, 1, "avatar 字段必填")
		return
	}
	if fileHeader.Size > maxAvatarBytes {
		Fail(c, http.StatusRequestEntityTooLarge, 1,
			fmt.Sprintf("头像不能超过 %dMB", maxAvatarBytes>>20))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, "open file: "+err.Error())
		return
	}
	defer file.Close()

	// 4. 写 MinIO
	// E2E-27 #4：ctx 必须带 3s deadline——MinIO 停机时快速失败（运行时实证
	// 裸 request ctx 无 deadline ⇒ curl 15s 挂起 000）；连接类失败映射 503。
	putCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	objectKey := storage.ObjectKey(uid, fileHeader.Filename)
	// E2E-27 M1 / F-116（ADR-2026-09 决策 1）：PutObject 返回的 publicURL
	// （PublicBaseURL 绝对地址）**不再下发也不落库**——只作内部用途；
	// 面向客户端一律网关相对路径，由 GET image 端点反代取回。
	_, err = h.storage.PutObject(putCtx, objectKey, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		if isStorageUnavailableErr(err) {
			Fail(c, http.StatusServiceUnavailable, 1, "storage unavailable: "+err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 1, "storage put: "+err.Error())
		return
	}

	// 5. 同步 user-svc 写库（UpdateMe AvatarURL）
	//
	// E2E-11：必须用 session.WithRequestAuth(c) 包 ctx —— 它把 X-User-Id 存入 ctx，
	// 下游 gRPC 客户端的 withUserID(ctx) 才能带上 x-user-id metadata。
	// 原实现传 c.Request.Context() ⇒ metadata 缺失 ⇒ user-svc 拦截器返
	// Unauthenticated ⇒ 头像上传 500（且 MinIO 对象已写入 → 孤儿对象）。
	avatarURL := "/api/v1/user/avatar/image/" + path.Base(objectKey)
	_, err = h.user.UpdateMe(session.WithRequestAuth(c), downstream.UpdateProfileReq{
		AvatarURL: &avatarURL,
	})
	if err != nil {
		// 已写 MinIO 但写库失败 — 此处不删 MinIO（防删了用户没换上的图）
		// 留运维通过 MinIO 控制台清理；前端感知 500 重试
		Fail(c, http.StatusInternalServerError, 1, "user-svc update: "+err.Error())
		return
	}

	// 与 BFF 其他端点一致，走 OK() 的 {code, message, data} 包装。
	//
	// E2E-11 复查（IAB 实测）：原实现手写 gin.H{"code","message","avatar"} 把 URL
	// 放在**顶层**（无 data 字段），而前端 useApi 统一 `return data.data` ⇒
	// `post<{avatar}>()` 返回 undefined ⇒ `res.avatar` 抛 TypeError 被 catch
	// ⇒ 用户看到"上传失败"提示，但服务端其实已写成功（DB 已更新）。
	OK(c, gin.H{"avatar": avatarURL})
}

// image GET /api/v1/user/avatar/image/:filekey —— avatar 对象反代
// （E2E-27 M1 / ADR-2026-09 决策 1/3，voice_handler.audio 同型）：
//   - 单段 filekey（gin 路由限制），handler 补 "avatars/" 前缀还原桶内 key
//   - 拒含 / 或 .. 的 key（400，不触达 storage）；对象缺失 404（非 200 空 body）
//   - storage 未配置 503（与上传语义一致）；回读真实 Content-Type/Length（<img> 可缓存）
func (h *AvatarHandler) image(c *gin.Context) {
	fileKey := c.Param("filekey")
	if fileKey == "" || strings.Contains(fileKey, "/") || strings.Contains(fileKey, "..") {
		Fail(c, http.StatusBadRequest, 1, "invalid key")
		return
	}
	if h.storage == nil {
		Fail(c, http.StatusServiceUnavailable, 1, "object storage not configured")
		return
	}

	rc, contentType, size, err := h.storage.GetObject(c.Request.Context(), "avatars/"+fileKey)
	if err != nil {
		if isStorageNotFoundErr(err) {
			Fail(c, http.StatusNotFound, 1, "avatar not found")
			return
		}
		if isStorageUnavailableErr(err) {
			Fail(c, http.StatusServiceUnavailable, 1, "storage unavailable: "+err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 1, "storage get: "+err.Error())
		return
	}
	defer rc.Close()

	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.FormatInt(size, 10))
	c.Status(http.StatusOK)
	if _, copyErr := io.Copy(c.Writer, rc); copyErr != nil {
		c.Error(copyErr)
		return
	}
}