// Package discovery — Round 4.2 BeatInstance 真正实现
//
// 背景：nacos-sdk-go v2.3.5 无公开 BeatInstance/SendHeartbeat API。
// 当前 Heartbeat() 退化为 UpdateInstance（每次重写整个 instance），
// Nacos 服务端 5s 不发就过期，但 SDK 维护的客户端长连接是另一条通道。
//
// 修复：直接 HTTP POST /nacos/v1/ns/instance/beat（与 Java 客户端一致路径），
// 跳过 SDK 限制，按 BeatInstance 标准协议发送。
//
// BeatInstance 协议（Nacos 2.x OpenAPI）：
//   POST {server}/nacos/v1/ns/instance/beat
//   Params:
//     ip, port, serviceName, groupName, namespaceId, beat (JSON string of BeatInfo)
//   BeatInfo: { clusterName, ip, port, weight, metadata }
//   Response: 200 OK {"clientBeatInterval": 5000}
//
// 设计：保留现有 UpdateInstance 兜底（SDK 长连接断开时 fallback），新增 HTTP beat
// 作为主通道。失败 → 退化为 UpdateInstance（向后兼容）。
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	nacosvo "github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// beatIntervalMS 默认 5s（与 Nacos 服务端默认一致）；SDK BeatInstance 响应
// 会带 clientBeatInterval 字段，本实现固定 5s（Nacos 2.x 服务端也接受此间隔）。
const defaultBeatIntervalMS = 5000

// BeatInfo 是 Nacos /instance/beat 端点的请求体（与 Java 客户端字段一致）。
//
// Round 4.2 P1-7 修复：原 UpdateInstance 心跳方式 30s 后过期，
// 改走 BeatInstance 标准协议（轻量级，server 不重新分配 instance）。
type BeatInfo struct {
	ClusterName string            `json:"clusterName"`
	IP          string            `json:"ip"`
	Port        uint64            `json:"port"`
	Weight      float64           `json:"weight"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// BeatResponse 是 Nacos /instance/beat 端点的响应。
// clientBeatIntervalMS 是服务端建议的下次心跳间隔。
type BeatResponse struct {
	ClientBeatIntervalMS int `json:"clientBeatInterval"`
}

// sendBeatInstance HTTP POST /nacos/v1/ns/instance/beat，发送单次心跳。
//
// 失败返回 error（不 panic，让 caller 决定退化为 UpdateInstance 还是 fail-fast）。
func sendBeatInstance(
	ctx context.Context,
	serverAddr, namespaceID, groupName, serviceName string,
	beat BeatInfo,
) (time.Duration, error) {
	// 多个 serverAddr 逗号分隔 → 取第一个（与 SDK client_factory 一致行为）
	serverAddr = strings.TrimSpace(serverAddr)
	if serverAddr == "" {
		return 0, fmt.Errorf("empty serverAddr")
	}
	servers := strings.Split(serverAddr, ",")
	addr := strings.TrimSpace(servers[0])
	// 如果 addr 已带 scheme（如 http://host:port），不再加；否则补 http://
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}

	beatJSON, err := json.Marshal(beat)
	if err != nil {
		return 0, fmt.Errorf("marshal beat: %w", err)
	}

	form := url.Values{}
	form.Set("beat", string(beatJSON))
	form.Set("serviceName", serviceName)
	form.Set("groupName", groupName)
	form.Set("namespaceId", namespaceID)
	form.Set("ip", beat.IP)
	form.Set("port", fmt.Sprintf("%d", beat.Port))

	// 端点：{scheme}://{host}/nacos/v1/ns/instance/beat
	// URL.Path 总是带前导 "/"——直接拼字符串更可靠
	endpoint := strings.TrimRight(addr, "/") + "/nacos/v1/ns/instance/beat"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send beat: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("beat HTTP %d: %s", resp.StatusCode, string(body))
	}

	var beatResp BeatResponse
	if err := json.Unmarshal(body, &beatResp); err != nil {
		// 响应解析失败时用默认间隔
		return time.Duration(defaultBeatIntervalMS) * time.Millisecond, nil
	}
	if beatResp.ClientBeatIntervalMS <= 0 {
		return time.Duration(defaultBeatIntervalMS) * time.Millisecond, nil
	}
	return time.Duration(beatResp.ClientBeatIntervalMS) * time.Millisecond, nil
}

// BeatHeartbeat 启动 BeatInstance 协议的后台 watcher（Round 4.2 P1-7 修复）。
//
// 行为：
//   - 按服务端返回的 clientBeatIntervalMS 周期发送 beat
//   - 失败时退化为 SDK UpdateInstance（兜底——SDK 长连接可能仍存活）
//   - ctx.Done() 时退出 goroutine
//
// 优先级：高（覆盖 SDK Heartbeat 调用方——所有 Nacos 注册方都改用此入口）
func (r *NacosRegistry) BeatHeartbeat(ctx context.Context, ins Instance, initialInterval time.Duration) {
	if initialInterval <= 0 {
		initialInterval = 5 * time.Second
	}
	// BeatInstance 协议 metadata 字段（与 Java 客户端一致）
	beat := BeatInfo{
		ClusterName: "DEFAULT",
		IP:          registerHost(ins.Host),
		Port:        uint64(ins.Port),
		Weight:      1.0,
		Metadata:    ins.Metadata,
	}

	go func() {
		interval := initialInterval
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		failCount := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nextInterval, err := sendBeatInstance(
					ctx,
					r.cfg.ServerAddr,
					r.cfg.Namespace,
					r.cfg.GroupName,
					ins.ServiceName,
					beat,
				)
				if err != nil {
					failCount++
					// 失败兜底：SDK UpdateInstance（保持长连接）
					if failCount <= 3 {
						slog.WarnContext(ctx, "nacos BeatInstance failed, fallback to SDK UpdateInstance",
							"err", err, "fail_count", failCount)
					}
					_, _ = r.client.UpdateInstance(nacosvo.UpdateInstanceParam{
						Ip:          beat.IP,
						Port:        beat.Port,
						Weight:      1.0,
						Enable:      true,
						Healthy:     true,
						Metadata:    beat.Metadata,
						ClusterName: beat.ClusterName,
						ServiceName: ins.ServiceName,
						GroupName:   r.cfg.GroupName,
						Ephemeral:   true,
					})
					// 兜底后维持初始 interval（不要被 failCount 拉长间隔）
					continue
				}
				if failCount > 0 {
					slog.InfoContext(ctx, "nacos BeatInstance recovered", "fail_count", failCount)
					failCount = 0
				}
				if nextInterval != interval && nextInterval > 0 {
					ticker.Reset(nextInterval)
					interval = nextInterval
				}
			}
		}
	}()
}

// _ = constant 防止 import 警告（保留扩展点——未来如 SDK 升级到 v2.4+ 支持 BeatInstance，
// 可在此引入 SDK BeatInstanceParam 替代 HTTP 实现）
var _ = constant.ClientConfig{}
