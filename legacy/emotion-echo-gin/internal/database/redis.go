package database

import (
	"context"
	"fmt"
	"log"

	"emotion-echo-gin/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedis 创建 Redis 连接
func NewRedis(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.GetRedisAddr(),
		Password: cfg.Database.Redis.Password,
		DB:       cfg.Database.Redis.DB,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	log.Println("Redis connected")
	return client, nil
}
