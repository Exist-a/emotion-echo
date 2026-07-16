package errors

// 业务错误码（与 API_DESIGN.md v2.0 一致）
const (
	// 通用错误 10000-10099
	ErrInvalidParams   = 10001
	ErrTokenExpired    = 10002
	ErrTokenInvalid    = 10003
	ErrForbidden       = 10004
	ErrTooManyRequests = 10005

	// 用户错误 20000-20099
	ErrUserNotFound      = 20001
	ErrPasswordIncorrect = 20002
	ErrInvalidVerifyCode = 20003
	ErrUserExists        = 20004
	ErrVerifyCodeLimit   = 20005

	// 会话错误 30000-30099
	ErrConversationNotFound = 30001
	ErrNotConversationOwner = 30002

	// AI 错误 40000-40099
	ErrAIServiceBusy   = 40001
	ErrAIServiceFailed = 40002

	// 系统错误 50000-50099
	ErrInternalServer = 50001
)

// 错误消息映射
var messages = map[int]string{
	ErrInvalidParams:        "参数错误",
	ErrTokenExpired:         "Token 已过期",
	ErrTokenInvalid:         "Token 无效",
	ErrForbidden:            "权限不足",
	ErrTooManyRequests:      "请求过于频繁",
	ErrUserNotFound:         "用户不存在",
	ErrPasswordIncorrect:    "密码错误",
	ErrInvalidVerifyCode:    "验证码错误或已过期",
	ErrUserExists:           "用户已存在",
	ErrVerifyCodeLimit:      "验证码发送过于频繁",
	ErrConversationNotFound: "会话不存在",
	ErrNotConversationOwner: "非会话所有者",
	ErrAIServiceBusy:        "AI 服务繁忙",
	ErrAIServiceFailed:      "AI 服务调用失败",
	ErrInternalServer:       "服务器内部错误",
}

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}

// New 创建业务错误
func New(code int, details ...string) *BusinessError {
	msg := messages[code]
	if msg == "" {
		msg = "未知错误"
	}
	if len(details) > 0 {
		msg = msg + "：" + details[0]
	}
	return &BusinessError{
		Code:    code,
		Message: msg,
	}
}

// GetMessage 获取错误消息
func GetMessage(code int) string {
	if msg, ok := messages[code]; ok {
		return msg
	}
	return "未知错误"
}

// IsBusinessError 判断是否为业务错误
func IsBusinessError(err error) (*BusinessError, bool) {
	if be, ok := err.(*BusinessError); ok {
		return be, true
	}
	return nil, false
}
