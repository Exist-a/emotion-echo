package handler

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/nanoid"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// GetProfile 获取用户信息
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := c.GetInt64("userId")

	user, err := h.userService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, user)
}

// UpdateProfile 更新用户信息
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetInt64("userId")

	var req service.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	if err := h.userService.UpdateProfile(c.Request.Context(), userID, &req); err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, nil)
}

// UploadAvatar 上传头像
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	userID := c.GetInt64("userId")

	// 1. 获取上传文件
	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "请上传头像文件")
		return
	}
	defer file.Close()

	// 2. 限制文件大小（2MB）
	const maxSize = 2 * 1024 * 1024
	if header.Size > maxSize {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "头像文件不能超过2MB")
		return
	}

	// 3. 验证文件类型（扩展名 + 魔数）
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	if !allowedExts[ext] {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "仅支持 jpg/png/webp 格式")
		return
	}

	// 读取文件头部进行魔数验证
	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "文件读取失败")
		return
	}

	if !isValidImageMagicNumber(head[:n], ext) {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "文件内容不符合图片格式")
		return
	}

	// 重置文件指针到开头
	if _, err := file.Seek(0, 0); err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer, "文件处理失败")
		return
	}

	// 4. 生成唯一文件名
	filename := nanoid.Generate() + ext
	savePath := filepath.Join("uploads", "avatars", filename)

	// 5. 保存文件
	out, err := os.Create(savePath)
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer, "文件保存失败")
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer, "文件写入失败")
		return
	}

	// 6. 更新用户头像
	avatarURL := fmt.Sprintf("/uploads/avatars/%s", filename)
	if err := h.userService.UpdateAvatar(c.Request.Context(), userID, avatarURL); err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, gin.H{"avatar": avatarURL})
}

// isValidImageMagicNumber 验证图片文件魔数
func isValidImageMagicNumber(head []byte, ext string) bool {
	if len(head) < 4 {
		return false
	}

	switch ext {
	case ".jpg", ".jpeg":
		// JPEG: FF D8 FF
		return len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF
	case ".png":
		// PNG: 89 50 4E 47
		return bytes.HasPrefix(head, []byte{0x89, 0x50, 0x4E, 0x47})
	case ".webp":
		// WebP: RIFF....WEBP
		return len(head) >= 12 &&
			bytes.HasPrefix(head, []byte("RIFF")) &&
			bytes.Equal(head[8:12], []byte("WEBP"))
	default:
		return false
	}
}
