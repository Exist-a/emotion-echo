package configcenter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	nacosclients "github.com/nacos-group/nacos-sdk-go/v2/clients"
	nacosconfig "github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	nacosconstant "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	nacosvo "github.com/nacos-group/nacos-sdk-go/v2/vo"

	"github.com/emotion-echo/shared/pkg/discovery"
)

// NacosConfig 是 ConfigCenter interface 的 Nacos 实现。
//
// 安全约束（ADR 决策 10 / 包级文档）：
//   - 不允许通过 PublishConfig 推送任何"敏感前缀"dataId（jwt.* / database.* / db.*
//     / kafka.* / llm.* / openai.* / deepseek.* / *.secret / *.password / *.token）。
//   - 此类配置必须留在 etc/*.yaml 或环境变量。
//   - GetConfig / ListenConfig 不做前缀过滤——允许读取 Nacos 已有的敏感项是
//     dev/prod 迁移期的现实需要，但调用方业务代码**禁止**读 jwt.* 等前缀。
type NacosConfig struct {
	client nacosconfig.IConfigClient
	cfg    discovery.NacosConfig

	mu        sync.Mutex
	listeners map[string]context.CancelFunc // key=dataId@group → cancel ongoing listen
	closed    bool
}

// 敏感 dataId 前缀（lowercase 比较）。
// 实现"防御性默认"：PublishConfig 会拒绝这些前缀，避免误操作泄漏到 Nacos。
var sensitivePrefixes = []string{
	"jwt.",
	"database.",
	"db.",
	"kafka.",
	"kafka_brokers",
	"llm.",
	"openai.",
	"deepseek.",
	"postgres_password",
}

var sensitiveSuffixes = []string{
	".secret",
	".password",
	".token",
	".dsn",
}

func isSensitiveDataId(dataId string) bool {
	low := strings.ToLower(dataId)
	for _, p := range sensitivePrefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	for _, s := range sensitiveSuffixes {
		if strings.HasSuffix(low, s) {
			return true
		}
	}
	return false
}

// NewNacosConfig 用与 NewNacosRegistry 相同的 NacosConfig 连接 Nacos 配置中心。
//
// 复用 discovery.NacosConfig（ServerAddr / Namespace / GroupName）保持两个客户端
// 的连接参数一致。
func NewNacosConfig(ctx context.Context, cfg discovery.NacosConfig) (*NacosConfig, error) {
	discovery.ApplyDefaults(&cfg)

	serverConfigs, err := buildServerConfigsForCC(cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("configcenter: parse nacos server addr: %w", err)
	}

	clientConfig := nacosconstant.NewClientConfig(
		nacosconstant.WithNamespaceId(cfg.Namespace),
		nacosconstant.WithTimeoutMs(cfg.TimeoutMs),
		nacosconstant.WithNotLoadCacheAtStart(true),
		nacosconstant.WithUpdateThreadNum(2),
	)

	client, err := nacosclients.NewConfigClient(nacosvo.NacosClientParam{
		ClientConfig:  clientConfig,
		ServerConfigs: serverConfigs,
	})
	if err != nil {
		return nil, fmt.Errorf("configcenter: create nacos config client: %w", err)
	}

	return &NacosConfig{
		client:    client,
		cfg:       cfg,
		listeners: make(map[string]context.CancelFunc),
	}, nil
}

// -----------------------------------------------------------------------------
// ConfigCenter interface implementation
// -----------------------------------------------------------------------------

func (c *NacosConfig) GetConfig(ctx context.Context, dataId, group string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if dataId == "" {
		return "", errors.New("configcenter: empty dataId")
	}
	content, err := c.client.GetConfig(nacosvo.ConfigParam{
		DataId: dataId,
		Group:  groupOrDefault(group, c.cfg.GroupName),
	})
	if err != nil {
		return "", fmt.Errorf("configcenter: get %s/%s: %w", group, dataId, err)
	}
	return content, nil
}

func (c *NacosConfig) ListenConfig(ctx context.Context, dataId, group string, handler ConfigChangeHandler) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dataId == "" {
		return errors.New("configcenter: empty dataId")
	}
	if handler == nil {
		return errors.New("configcenter: nil handler")
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("configcenter: client already closed")
	}
	// 替换前一个同 key 的 listener（契约：重复注册到同 (dataId, group) 应替换前一个）
	if cancel, ok := c.listeners[keyOfCC(dataId, group)]; ok {
		cancel()
	}
	listenCtx, cancel := context.WithCancel(context.Background())
	c.listeners[keyOfCC(dataId, group)] = cancel
	c.mu.Unlock()

	group = groupOrDefault(group, c.cfg.GroupName)

	param := nacosvo.ConfigParam{
		DataId: dataId,
		Group:  group,
		OnChange: func(_, _, _, content string) {
			// Nacos SDK 在独立 goroutine 中回调；如遇 handler error 仅记录，
			// 不取消订阅（订阅生命周期独立于 handler 业务结果）。
			if err := handler(dataId, group, content); err != nil {
				// 用 fmt 替代 log：避免引入 zerolog 等具体日志实现，保持 shared 轻量。
				fmt.Printf("[configcenter] handler error on %s/%s: %v\n", group, dataId, err)
			}
		},
	}
	if err := c.client.ListenConfig(param); err != nil {
		cancel()
		// 回滚 listeners 表
		c.mu.Lock()
		delete(c.listeners, keyOfCC(dataId, group))
		c.mu.Unlock()
		return fmt.Errorf("configcenter: listen %s/%s: %w", group, dataId, err)
	}

	// 当 listenCtx 被 cancel 时调 CancelListenConfig 释放 SDK 资源
	go func() {
		<-listenCtx.Done()
		_ = c.client.CancelListenConfig(param)
	}()

	return nil
}

func (c *NacosConfig) PublishConfig(ctx context.Context, dataId, group, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dataId == "" {
		return errors.New("configcenter: empty dataId")
	}
	if isSensitiveDataId(dataId) {
		return fmt.Errorf("configcenter: refusing to publish sensitive dataId %q "+
			"(must be set via etc/*.yaml or env)", dataId)
	}

	ok, err := c.client.PublishConfig(nacosvo.ConfigParam{
		DataId:  dataId,
		Group:   groupOrDefault(group, c.cfg.GroupName),
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("configcenter: publish %s/%s: %w", group, dataId, err)
	}
	if !ok {
		return fmt.Errorf("configcenter: publish %s/%s: nacos returned not-ok", group, dataId)
	}
	return nil
}

func (c *NacosConfig) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for _, cancel := range c.listeners {
		cancel()
	}
	c.listeners = make(map[string]context.CancelFunc)
	c.mu.Unlock()

	// SDK 提供 CloseClient()，用于释放长连接；无错误返回。
	c.client.CloseClient()
	return nil
}

// 编译期断言：NacosConfig 必须实现 ConfigCenter interface。
var _ ConfigCenter = (*NacosConfig)(nil)

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func keyOfCC(dataId, group string) string { return dataId + "@" + group }

func groupOrDefault(group, fallback string) string {
	if group == "" {
		return fallback
	}
	return group
}

// buildServerConfigsForCC 复用 discovery 包内同语义函数，避免重复实现。
//
// 通过在 discovery 包内暴露 BuildServerConfigs（PR-03 的 buildServerConfigs 已存在，
// 这里直接调用）。等价性测试在 nacos_config_test.go 中验证。
func buildServerConfigsForCC(addr string) ([]nacosconstant.ServerConfig, error) {
	return buildServerConfigsInline(addr)
}

// buildServerConfigsInline 是 buildServerConfigs 的内联等价实现，避免
// configcenter 包 import discovery 引起循环依赖（discovery 可能反过来引用 configcenter）。
func buildServerConfigsInline(addr string) ([]nacosconstant.ServerConfig, error) {
	if addr == "" {
		return nil, errors.New("empty nacos server addr")
	}
	parts := strings.Split(addr, ",")
	out := make([]nacosconstant.ServerConfig, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		host, portStr, err := net.SplitHostPort(p)
		if err != nil {
			return nil, fmt.Errorf("invalid addr %q: %w", p, err)
		}
		port, err := strconv.ParseUint(portStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
		}
		out = append(out, nacosconstant.ServerConfig{IpAddr: host, Port: port})
	}
	return out, nil
}
