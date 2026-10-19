// Package memorykeys 定义跨服务共享的 Redis 键与 TTL。
//
// 为什么放在 shared：写侧在 ai-svc、读侧在 web-bff —— 两端各写一份字符串
// 必然漂移（本仓已有的教训：`CL-F-48` 两套 JSON tag）。这里一处定义、两端引用。
package memorykeys

import (
	"strconv"
	"time"
)

// SummaryPrefix 用户级记忆摘要的键前缀。
//
// 键形如 `memory:summary:user:42`；值为纯文本（逐行 "- <时间> <事实>"）。
const SummaryPrefix = "memory:summary:user:"

// SummaryTTL 摘要 TTL —— 与 `CL-D-07`（融合情绪用户级热点）同构：24h。
//
// 过期后不影响对话（注入侧取不到即降级为空）；下一次抽取会重建。
const SummaryTTL = 24 * time.Hour

// SummaryKey 返回某用户的摘要键。
func SummaryKey(userID int64) string {
	return SummaryPrefix + strconv.FormatInt(userID, 10)
}
