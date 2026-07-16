package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
)

// Response 统一响应结构（与 v2.0 API 兼容）
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// Error 错误响应
func Error(c *gin.Context, code int, message string) {
	c.JSON(getHTTPStatus(code), Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// ErrorWithCode 使用错误码响应
func ErrorWithCode(c *gin.Context, code int, details ...string) {
	msg := errors.GetMessage(code)
	if len(details) > 0 {
		msg = msg + "：" + details[0]
	}
	Error(c, code, msg)
}

// ErrorFromBusinessError 从业务错误响应
func ErrorFromBusinessError(c *gin.Context, err *errors.BusinessError) {
	Error(c, err.Code, err.Message)
}

// getHTTPStatus 获取 HTTP 状态码
func getHTTPStatus(code int) int {
	switch {
	case code == 0:
		return http.StatusOK
	case code == 10001:
		return http.StatusBadRequest // 参数错误
	case code == 10002 || code == 10003:
		return http.StatusUnauthorized // Token 过期/无效
	case code == 10004:
		return http.StatusForbidden // 权限不足
	case code == 10005:
		return http.StatusTooManyRequests // 请求过于频繁
	case code >= 20001 && code <= 20005:
		// 登录相关错误返回 200，前端根据 code 处理
		return http.StatusOK
	case code == 30001:
		return http.StatusNotFound
	case code == 30002:
		return http.StatusForbidden
	case code >= 40001 && code <= 40002:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
