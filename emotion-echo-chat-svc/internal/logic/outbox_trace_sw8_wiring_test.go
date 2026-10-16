package logic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== CL-03 Phase 2 caller-wiring 契约（写侧）=====
//
// 为什么用 source 断言而不是行为测试（如实说明，避免"看着像行为测试"的假证据）：
// 写 outbox 行的路径要求 `svcCtx.DB != nil`（真实 *gorm.DB），而本站单测体系没有
// 内存 gorm 驱动（go.mod 只有 postgres 驱动 + testcontainers；后者需 Docker）。
// 仓内同型先例：createconversationlogic_test.go:141 与
// outbox.TestCleanupOnce_CallerWiringInMainGo 都用 source 断言钉"调用点接线"。
// 真正的端到端行为（列里真有 sw8）由 Phase 7 起栈验证（🔴 需栈）覆盖。

// readLogicSource 读同目录下的实现文件源码。
func readLogicSource(t *testing.T, name string) string {
	t.Helper()
	bs, err := os.ReadFile(filepath.Join(".", name))
	require.NoErrorf(t, err, "读不到 %s（测试须在 internal/logic 目录下运行）", name)
	return string(bs)
}

// assertOutboxRowCarriesTraceSW8 断言 name 里**每一处** CreateInTx 构造的 OutboxEvent
// 都带 TraceSW8 字段（否则那条路径的 trace 会断在 outbox→relay）。
func assertOutboxRowCarriesTraceSW8(t *testing.T, name string) {
	t.Helper()
	src := readLogicSource(t, name)

	// 逐块切出 CreateInTx( ... ) 的字面量，检查每块都含 TraceSW8:
	parts := strings.Split(src, "OutboxRepo.CreateInTx(")
	require.GreaterOrEqual(t, len(parts), 2, "%s 里找不到 OutboxRepo.CreateInTx 调用点", name)

	for i, p := range parts[1:] {
		// 取该调用的字面量片段（到第二个 "})" 或 2000 字符为止，足够覆盖结构体字面量）
		seg := p
		if len(seg) > 2000 {
			seg = seg[:2000]
		}
		assert.Containsf(t, seg, "TraceSW8:",
			"%s 第 %d 处 CreateInTx 未设置 TraceSW8 —— CL-03 Phase 2 要求写入 outbox 行时"+
				"存下当前 ctx 的 sw8（Phase 3 的 relay 靠它恢复父 ctx）", name, i+1)
	}
}

// TestSendMessageLogic_WritesTraceSW8IntoOutboxRow 发消息路径（最热路径）。
func TestSendMessageLogic_WritesTraceSW8IntoOutboxRow(t *testing.T) {
	assertOutboxRowCarriesTraceSW8(t, "sendmessagelogic.go")
}

// TestCreateConversationLogic_WritesTraceSW8IntoOutboxRow 建会话路径。
func TestCreateConversationLogic_WritesTraceSW8IntoOutboxRow(t *testing.T) {
	assertOutboxRowCarriesTraceSW8(t, "createconversationlogic.go")
}

// TestDeleteConversationLogic_WritesTraceSW8IntoOutboxRow 删会话路径（含两处 CreateInTx）。
func TestDeleteConversationLogic_WritesTraceSW8IntoOutboxRow(t *testing.T) {
	assertOutboxRowCarriesTraceSW8(t, "deleteconversationlogic.go")
}

// TestOutboxWriteSites_UseEncodeTraceSW8Helper 断言调用点用的是统一 helper
// （而不是各自拼 tracer），避免三处实现漂移。
func TestOutboxWriteSites_UseEncodeTraceSW8Helper(t *testing.T) {
	for _, name := range []string{"sendmessagelogic.go", "createconversationlogic.go", "deleteconversationlogic.go"} {
		assert.Containsf(t, readLogicSource(t, name), "outbox.EncodeTraceSW8(",
			"%s 应通过 outbox.EncodeTraceSW8 取 sw8（统一 helper，避免三处漂移）", name)
	}
}
