// Package password 提供统一的密码哈希与校验工具。
//
// Stage 33 PR-19a：从 legacy/emotion-echo-gin/internal/pkg/password 抽离
// 到共享包，让 user-svc 及其他未来需要密码能力的 svc 直接 import。
//
// 实现选用 bcrypt（golang.org/x/crypto/bcrypt，DefaultCost = 10）：
//   - 抗暴力破解：自适应 cost 因子
//   - 自动加盐：相同密码每次哈希结果不同
//   - 标准库背书：跨语言生态兼容（PHP/Java/Python 都有 bcrypt 实现）
//
// 不做"前端 sha256(password) 再传"的链式哈希（bcrypt(sha256) 等价于
// bcrypt 明文但削弱了 bcrypt 的安全性 —— 攻击者只需对 sha256 值做字典
// 攻击）。前端应直接传明文密码，dev compose 走 profile: tls 才能安全。
package password

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrEmptyPassword 当明文密码为空字符串时返回。
//
// bcrypt 本身对极短密码（如 1 字节）也会接受；这里额外拦截空字符串，
// 因为"空密码"在业务语义上 100% 是错误输入（不应被哈希入库）。
var ErrEmptyPassword = errors.New("password: empty plain password")

// Hash 把明文密码哈希为 bcrypt 字符串（含算法前缀、cost、盐、密文）。
//
// 返回值特征：
//   - 长度固定 60 字节
//   - 格式 \$2a\$10\$<22字符盐><31字符密文>
func Hash(plain string) (string, error) {
	if plain == "" {
		return "", ErrEmptyPassword
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Verify 校验明文密码与哈希值是否匹配。
//
// 用于登录场景：拿到用户表里的 password_hash 与用户提交的明文对比。
func Verify(plain, hash string) bool {
	if plain == "" || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
