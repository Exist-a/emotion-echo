package main

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-chat-svc/internal/outbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 测试点 #24：ops 配置**首帧拉取失败**时服务必须继续启动。
//
// 语义依据（shared/pkg/configcenter/nacos_config.go 包文档）：
//   "GetConfig / ListenConfig 不做前缀过滤 —— 允许读取 Nacos 已有的敏感项是
//    dev/prod 迁移期的现实需要"，且 `IsHardBootError` 把 `Register` /
//   `WaitForNacos` 归为 hard（必须 fail-fast），而 **GetConfig 不在其列**
//   ⇒ 配置中心不可用时，Nacos 里没有 ops 配置是**正常状态**（dataId 尚未
//   被创建），不是故障。
//
// 若这里改成 fail-fast，会导致"运维还没推过 ops 配置"的新环境直接起不来 ——
// 那是把"缺配置"误判成"依赖故障"。
//
// 本测试锁定：GetConfig 报错 → BootNacos 仍成功、注册照常、ops 保持启动值。

func TestBootNacos_OpsGetConfigFailure_StillBoots(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.getErr = errors.New("config data not exist")

	ops := outbox.NewOps(outbox.OpsConfig{MaxAttempts: 100, SentRetentionDays: 7})
	deps := newDeps(reg, cc)
	deps.ops = ops

	rt, err := BootNacos(context.Background(), newTestConfig(), deps)
	require.NoError(t, err, "ops 配置拉不到**不应**阻止启动 —— 缺配置是正常状态")
	require.NotNil(t, rt)

	// 注册必须照常发生（缺 ops 配置不影响服务发现）
	assert.Len(t, reg.registered, 1, "服务注册不应受 ops 配置拉取失败影响")

	// ops 保持启动时的值（不是被 0 覆盖）
	snap := ops.Snapshot()
	assert.Equal(t, 100, snap.MaxAttempts, "首帧失败后 ops 应保持启动值")
	assert.Equal(t, 7, snap.SentRetentionDays, "首帧失败后 ops 应保持启动值")
}

// TestBootNacos_OpsAppliedOnFirstFrame 与 #24 配对：拉得到就必须真应用。
func TestBootNacos_OpsAppliedOnFirstFrame(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.configs["emotion-echo-chat-svc.ops.yaml@DEFAULT_GROUP"] =
		"max_attempts: 42\nsent_retention_days: 3\ndead_retention_days: 5\ncleanup_interval_s: 900\n"

	ops := outbox.NewOps(outbox.OpsConfig{MaxAttempts: 100, SentRetentionDays: 7})
	deps := newDeps(reg, cc)
	deps.ops = ops

	_, err := BootNacos(context.Background(), newTestConfig(), deps)
	require.NoError(t, err)

	snap := ops.Snapshot()
	assert.Equal(t, 42, snap.MaxAttempts, "首帧拉到的值必须真正应用（不是只打日志）")
	assert.Equal(t, 3, snap.SentRetentionDays)
	assert.Equal(t, 5, snap.DeadRetentionDays)
	assert.Equal(t, 900, snap.CleanupIntervalS)
}

// TestBootNacos_OpsSensitiveKeysStripped 覆盖 P1：ops 里的敏感 key 被剔除。
func TestBootNacos_OpsSensitiveKeysStripped(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.configs["emotion-echo-chat-svc.ops.yaml@DEFAULT_GROUP"] =
		"max_attempts: 42\nllm.api_key: sk-should-never-apply\nauth_token: also-secret\n"

	ops := outbox.NewOps(outbox.OpsConfig{MaxAttempts: 100})
	deps := newDeps(reg, cc)
	deps.ops = ops

	_, err := BootNacos(context.Background(), newTestConfig(), deps)
	require.NoError(t, err)

	// 正常参数应用
	assert.Equal(t, 42, ops.Snapshot().MaxAttempts)
	// 敏感 key 不在 OpsConfig 的白名单结构体里，无论如何都不会被应用；
	// 清洗层再叠一道，保证原始内容里也不残留。
	assert.Equal(t, outbox.OpsSnapshot{
		MaxAttempts: 42, SentRetentionDays: 0, DeadRetentionDays: 0, CleanupIntervalS: 0,
	}, ops.Snapshot(), "只应有白名单字段被应用")
}

// TestBootNacos_OpsNotInjected_DegradesGracefully 未注入 ops 容器时退化为
// "只记录不应用"（F-155 修复前的行为），且不影响注册。
func TestBootNacos_OpsNotInjected_DegradesGracefully(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.configs["emotion-echo-chat-svc.ops.yaml@DEFAULT_GROUP"] = "max_attempts: 42\n"

	// deps.ops 保持 nil
	rt, err := BootNacos(context.Background(), newTestConfig(), newDeps(reg, cc))
	require.NoError(t, err, "未注入 ops 容器不应导致启动失败")
	require.NotNil(t, rt)
	assert.Len(t, reg.registered, 1, "注册照常")
}
