// Package storage — Sprint 1 PR-4b: BFF 对象存储抽象 + MinIO 实现
//
// 设计目标：
//   - handler 依赖接口（StorageClient）而非具体客户端（testability / 后续切换 OSS / S3）
//   - NewMinIOClient 工厂 + 暴露 PutObject / GetObjectURL / RemoveObject 三个常用操作
//   - 上传文件 key 命名约定：<bucket-prefix>/<uid>-<sha256-prefix>.<ext>
//     —— uid 隔离用户；sha256 prefix 防重复；不在 URL 暴露绝对路径
//
// 调研依据：
//   - emotion-echo-web-bff/internal/config/config.go（Config struct 扩展 MinIO 段）
//   - emotion-echo-web-bff/internal/svc/servicecontext.go（ServiceContext 注入）
//   - quay.io/minio/minio 官方 go SDK minio-go/v7
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// StorageClient 是 BFF handler 依赖的存储接口。
// 后续 PR-4c handler 用这个接口而非直接拿 *minio.Client，便于 fake / 切云厂商。
type StorageClient interface {
	// PutObject 上传 reader 内容到 bucket 的 objectKey，返回公开可访问 URL
	PutObject(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) (publicURL string, err error)
	// GetObjectURL 返 objectKey 的公开 URL（不含签名，依赖 bucket 设为 anonymous download）
	GetObjectURL(objectKey string) string
	// RemoveObject 删除对象（头像更新场景用）
	RemoveObject(ctx context.Context, objectKey string) error
	// HealthCheck 连通性检查（启动期 + healthz 用）
	HealthCheck(ctx context.Context) error
}

// MinIOConfig 是 NewMinIOClient 的配置（来自 Config.MinIO 段）
type MinIOConfig struct {
	Endpoint     string // e.g. "emotion-echo-minio:9000" (容器内) 或 "minio.example.com" (prod)
	AccessKey    string
	SecretKey    string
	Bucket       string // 必填：默认 avatars
	UseSSL       bool   // dev false，prod true
	PublicBaseURL string // 返给前端的 URL 前缀（dev: http://localhost:9000；prod: https://cdn.example.com）
}

// MinIOClient 是 MinIO 实现
type MinIOClient struct {
	cfg    MinIOConfig
	client *minio.Client
}

// NewMinIOClient 构造 MinIO 客户端；如 Bucket 为空则用 cfg.Bucket = "avatars"
func NewMinIOClient(cfg MinIOConfig) (*MinIOClient, error) {
	if cfg.Bucket == "" {
		cfg.Bucket = "avatars"
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}
	return &MinIOClient{cfg: cfg, client: cli}, nil
}

// ObjectKey 根据 uid + 原文件名生成稳定 key
//   - format: avatars/<uid>-<sha256(prefix)[:8]>.<ext>
//   - prefix 哈希原文件名避免重复；不暴露原文件名（用户隐私）
func ObjectKey(uid int64, originalFilename string) string {
	h := sha256.Sum256([]byte(originalFilename))
	prefix := hex.EncodeToString(h[:])[:8]
	ext := strings.ToLower(path.Ext(originalFilename))
	if ext == "" {
		ext = ".bin"
	}
	return fmt.Sprintf("avatars/%d-%s%s", uid, prefix, ext)
}

// PutObject 上传对象并返回公开 URL
func (m *MinIOClient) PutObject(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) (string, error) {
	// 验证 bucket 存在（dev 启动期 init 已建；prod 由 Helm 模板建）
	exists, err := m.client.BucketExists(ctx, m.cfg.Bucket)
	if err != nil {
		return "", fmt.Errorf("BucketExists(%s): %w", m.cfg.Bucket, err)
	}
	if !exists {
		return "", fmt.Errorf("bucket %s does not exist", m.cfg.Bucket)
	}

	info, err := m.client.PutObject(ctx, m.cfg.Bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("PutObject: %w", err)
	}
	_ = info // 忽略 ETag；后续可日志
	return m.GetObjectURL(objectKey), nil
}

// GetObjectURL 返回 objectKey 的公开 URL（依赖 bucket anonymous download）
// PublicBaseURL 已含 scheme（http/https），直接拼接 bucket + key
func (m *MinIOClient) GetObjectURL(objectKey string) string {
	base := strings.TrimRight(m.cfg.PublicBaseURL, "/")
	return fmt.Sprintf("%s/%s/%s", base, m.cfg.Bucket, objectKey)
}

// RemoveObject 删除对象
func (m *MinIOClient) RemoveObject(ctx context.Context, objectKey string) error {
	return m.client.RemoveObject(ctx, m.cfg.Bucket, objectKey, minio.RemoveObjectOptions{})
}

// HealthCheck 用 BucketExists + ListObjects（轻量）检查连通性
func (m *MinIOClient) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := m.client.BucketExists(ctx, m.cfg.Bucket)
	return err
}