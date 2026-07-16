package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"time"

	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/repository"
)

// VerifyService 验证码服务
type VerifyService struct {
	redisRepo *repository.RedisRepository
}

// NewVerifyService 创建验证码服务
func NewVerifyService(redisRepo *repository.RedisRepository) *VerifyService {
	return &VerifyService{redisRepo: redisRepo}
}

// SendVerifyCodeRequest 发送验证码请求
type SendVerifyCodeRequest struct {
	Username string `json:"username" binding:"required"`
	Type     string `json:"type" binding:"required,oneof=register login reset"`
}

// SendVerifyCode 发送验证码
func (s *VerifyService) SendVerifyCode(ctx context.Context, req *SendVerifyCodeRequest) error {
	// 1. 验证格式（手机号或邮箱）
	if !isValidPhone(req.Username) && !isValidEmail(req.Username) {
		return errors.New(errors.ErrInvalidParams, "请输入有效的手机号或邮箱")
	}

	// 2. 检查发送频率
	limited, err := s.redisRepo.CheckVerifyLimit(ctx, req.Type, req.Username)
	if err != nil {
		return err
	}
	if limited {
		return errors.New(errors.ErrVerifyCodeLimit)
	}

	// 3. 生成验证码
	code := generateVerifyCode()

	// 4. 存储验证码（5分钟有效），Key 按类型区分
	if err := s.redisRepo.SetVerifyCode(ctx, req.Type, req.Username, code, 5*time.Minute); err != nil {
		return err
	}

	// 5. 设置发送限制（60秒）
	if err := s.redisRepo.SetVerifyLimit(ctx, req.Type, req.Username, 60*time.Second); err != nil {
		return err
	}

	// 6. 发送验证码
	// 开发环境：验证码打印到控制台
	// 生产环境：需接入短信网关（阿里云/腾讯云）或邮件 SMTP 服务
	fmt.Printf("[验证码] %s: %s\n", req.Username, code)

	return nil
}

// generateVerifyCode 生成6位数字验证码（使用加密安全随机数）
func generateVerifyCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		// 回退方案：使用当前时间戳
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// isValidPhone 验证手机号
func isValidPhone(phone string) bool {
	pattern := `^1[3-9]\d{9}$`
	matched, _ := regexp.MatchString(pattern, phone)
	return matched
}

// isValidEmail 验证邮箱
func isValidEmail(email string) bool {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)
	return matched
}
