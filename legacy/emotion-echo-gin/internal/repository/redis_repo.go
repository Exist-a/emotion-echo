package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRepository Redis 数据访问
type RedisRepository struct {
	client *redis.Client
}

// NewRedisRepository 创建 Redis 仓库
func NewRedisRepository(client *redis.Client) *RedisRepository {
	return &RedisRepository{client: client}
}

// ===== 验证码相关 =====

// SetVerifyCode 设置验证码（key 按类型区分，避免 register/login/reset 互相覆盖）
func (r *RedisRepository) SetVerifyCode(ctx context.Context, codeType, username, code string, ttl time.Duration) error {
	key := fmt.Sprintf("verify:%s:%s", codeType, username)
	return r.client.Set(ctx, key, code, ttl).Err()
}

// GetVerifyCode 获取验证码
func (r *RedisRepository) GetVerifyCode(ctx context.Context, codeType, username string) (string, error) {
	key := fmt.Sprintf("verify:%s:%s", codeType, username)
	return r.client.Get(ctx, key).Result()
}

// DeleteVerifyCode 删除验证码
func (r *RedisRepository) DeleteVerifyCode(ctx context.Context, codeType, username string) error {
	key := fmt.Sprintf("verify:%s:%s", codeType, username)
	return r.client.Del(ctx, key).Err()
}

// SetVerifyLimit 设置验证码发送限制
func (r *RedisRepository) SetVerifyLimit(ctx context.Context, codeType, username string, ttl time.Duration) error {
	key := fmt.Sprintf("verify:limit:%s:%s", codeType, username)
	return r.client.Set(ctx, key, "1", ttl).Err()
}

// CheckVerifyLimit 检查验证码发送限制
func (r *RedisRepository) CheckVerifyLimit(ctx context.Context, codeType, username string) (bool, error) {
	key := fmt.Sprintf("verify:limit:%s:%s", codeType, username)
	exists, err := r.client.Exists(ctx, key).Result()
	return exists > 0, err
}

// ===== Token 黑名单相关 =====

// AddToBlacklist 添加 Token 到黑名单
func (r *RedisRepository) AddToBlacklist(ctx context.Context, token string, ttl time.Duration) error {
	hash := sha256Hash(token)
	key := fmt.Sprintf("blacklist:%s", hash)
	return r.client.Set(ctx, key, "1", ttl).Err()
}

// IsBlacklisted 检查 Token 是否在黑名单
func (r *RedisRepository) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	hash := sha256Hash(token)
	key := fmt.Sprintf("blacklist:%s", hash)
	exists, err := r.client.Exists(ctx, key).Result()
	return exists > 0, err
}

// ===== 限流相关 =====

// AllowRequest 检查是否允许请求（Token 桶算法）
func (r *RedisRepository) AllowRequest(ctx context.Context, key string, rate, burst int) (bool, error) {
	luaScript := `
        local key = KEYS[1]
        local rate = tonumber(ARGV[1])
        local burst = tonumber(ARGV[2])
        local now = tonumber(ARGV[3])
        
        local last_updated = redis.call('HGET', key, 'last_updated')
        local tokens = redis.call('HGET', key, 'tokens')
        
        if not last_updated then
            last_updated = now
            tokens = burst
        else
            last_updated = tonumber(last_updated)
            tokens = tonumber(tokens)
            local delta = math.max(0, now - last_updated)
            tokens = math.min(burst, tokens + delta * rate / 60)
        end
        
        if tokens >= 1 then
            tokens = tokens - 1
            redis.call('HSET', key, 'tokens', tokens)
            redis.call('HSET', key, 'last_updated', now)
            redis.call('EXPIRE', key, 60)
            return 1
        else
            redis.call('HSET', key, 'tokens', tokens)
            redis.call('HSET', key, 'last_updated', now)
            return 0
        end
    `

	now := time.Now().Unix()
	result, err := r.client.Eval(ctx, luaScript, []string{key}, rate, burst, now).Result()
	if err != nil {
		return false, err
	}
	return result.(int64) == 1, nil
}

// ===== 辅助函数 =====

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
