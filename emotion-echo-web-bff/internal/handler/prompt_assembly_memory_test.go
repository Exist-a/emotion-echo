// Package handler — prompt_assembly_memory_test.go
//
// CL-08a · A4：记忆段（BFF）的零回归 / 段序 / 预算 / 降级。
//
// 段序（canonical）：
//
//	[base] → [agent] → [personality] → [memory] → [emotion] → [guard]
//
// 两条硬约束（本文件就是它们的钉子）：
//   - 无记忆时输出**逐字等于**现状 golden（记忆为空 ⇒ 不产生段）；
//   - guard 恒末位；memory 必须排在 emotion **之前**（事实靠近前部、情绪信号靠后）。

package handler

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	sharedmemorykeys "github.com/emotion-echo/shared/pkg/memorykeys"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMemorySegment_Empty(t *testing.T) {
	for _, s := range []string{"", "   ", "\n\t"} {
		if got := buildMemorySegment(s); got != "" {
			t.Errorf("buildMemorySegment(%q) = %q, want 空串（不产生段）", s, got)
		}
	}
}

func TestBuildMemorySegment_IncludesPrefixAndBody(t *testing.T) {
	got := buildMemorySegment("- 2026-10 用户是后端工程师")
	if !strings.HasPrefix(got, memorySegmentPrefix) {
		t.Errorf("记忆段必须以固定前缀开头：%q", got)
	}
	if !strings.Contains(got, "用户是后端工程师") {
		t.Errorf("记忆段必须包含摘要正文：%q", got)
	}
}

func TestClampMemorySegment_Truncates(t *testing.T) {
	line := "- 2026-10 " + strings.Repeat("记忆", 800) // 1600+ runes，超 1200 上限
	out := clampMemorySegment(line)
	require.NotEqual(t, line, out, "超预算必须截断")
	assert.LessOrEqual(t, len([]rune(out)), maxMemorySegmentRunes+2, "截断后不得超过上限（允许省略号）")
	assert.True(t, strings.HasSuffix(out, "…"), "截断应有省略号提示")
}

func TestPromptAssembly_MemorySegmentOrder(t *testing.T) {
	h := &AIStreamHandler{}
	in := assembleInput{
		ctx:            context.Background(),
		faceEmotion:    "happy",
		faceConfidence: 0.9,
		agent:          goldenAgentProfile(),
		memory:         "- 2026-10 用户是后端工程师",
	}

	segs := h.systemSegments(in)
	kinds := make([]string, 0, len(segs))
	for _, s := range segs {
		kinds = append(kinds, s.Kind)
	}
	assert.Equal(t,
		[]string{segmentKindBase, segmentKindAgent, segmentKindMemory, segmentKindEmotion, segmentKindGuard},
		kinds,
		"段序不变量：memory 在 emotion 之前，guard 恒末位")
}

func TestPromptAssembly_NoMemory_ByteIdenticalToGolden(t *testing.T) {
	h := &AIStreamHandler{}
	in := assembleInput{ctx: context.Background()}

	got := h.assembleSystemPrompt(in)
	assert.Equal(t, goldenBase, got, "无记忆时输出必须逐字等于基线（零回归）")
}

// ---------------------------------------------------------------------------
// 降级：来源报错 / 无身份 ⇒ 空串（不阻断对话）
// ---------------------------------------------------------------------------

type fakeMemorySource struct {
	summary string
	err     error
}

func (f fakeMemorySource) LatestMemorySummary(ctx context.Context, userID int64) (string, error) {
	return f.summary, f.err
}

func TestLoadMemorySummary_Degrades(t *testing.T) {
	ctx := downstream.WithUserID(context.Background(), 7)

	t.Run("未装配 ⇒ 空", func(t *testing.T) {
		h := &AIStreamHandler{}
		assert.Equal(t, "", h.loadMemorySummary(ctx))
	})

	t.Run("来源报错 ⇒ 空", func(t *testing.T) {
		h := &AIStreamHandler{memory: fakeMemorySource{err: errors.New("redis down")}}
		assert.Equal(t, "", h.loadMemorySummary(ctx))
	})

	t.Run("无身份 ⇒ 空", func(t *testing.T) {
		h := &AIStreamHandler{memory: fakeMemorySource{summary: "x"}}
		assert.Equal(t, "", h.loadMemorySummary(context.Background()))
	})

	t.Run("正常 ⇒ 摘要", func(t *testing.T) {
		h := &AIStreamHandler{memory: fakeMemorySource{summary: "- 2026-10 用户是后端工程师"}}
		assert.Equal(t, "- 2026-10 用户是后端工程师", h.loadMemorySummary(ctx))
	})
}

// ---------------------------------------------------------------------------
// CL-08c · Phase 3：真实 Redis 源穿透 handler 的降级（收口 CL-F-70 ① 的
// handler 侧对偶——此前只有 fake source 验过；这里把真 redisMemorySummary
// 接进 AIStreamHandler 走 loadMemorySummary 的完整路径）。
// ---------------------------------------------------------------------------

func TestLoadMemorySummary_RealRedisSource_DegradesAndHits(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	ctx := downstream.WithUserID(context.Background(), 7)
	require.NoError(t, client.Set(ctx, sharedmemorykeys.SummaryKey(7), "- 2026-10 用户养了一只猫", 0).Err())

	hit := &AIStreamHandler{memory: downstream.NewRedisMemorySummary(client)}
	assert.Equal(t, "- 2026-10 用户养了一只猫", hit.loadMemorySummary(ctx),
		"命中 ⇒ 原文（真实源经 handler 全路径）")

	miss := &AIStreamHandler{memory: downstream.NewRedisMemorySummary(client)}
	assert.Equal(t, "", miss.loadMemorySummary(downstream.WithUserID(context.Background(), 8)),
		"未命中（真实 redis.Nil）⇒ 空串、不报错")
}

// T-8 钉：两条上游路径（llm-gRPC / HTTP 直连）都必须取记忆摘要 ——
// 只接一条路径时另一条静默无记忆（与 agent 段的同型钉）。
func TestAIStreamHandler_BothAssembleSites_LoadMemorySummary(t *testing.T) {
	src, err := os.ReadFile("ai_stream_handler.go")
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(src), "loadMemorySummary("),
		"两处 assembleInput 都要取记忆摘要（T-8：防单路径漂移）")
}
