//go:build integration
// +build integration

// Round 1.1 / P1-R2-8: voice_emotion_results.upload_id 列 UNIQUE 幂等修复
//
// 背景：i003 (voice_emotion_results) UNIQUE INDEX 原本 NULL 允许多行
// （PG 普通 UNIQUE 索引把多个 NULL 视为不重复）→ 同 voice 上传
// 多次可落 N 行，幂等失效。i006 已用 partial unique + __legacy__ 占位
// 模式修 face_emotion_results.upload_id 与 emotion_analysis.event_id。
// 本测试覆盖 voice_emotion_results.upload_id 同步修复（i008）。
//
// TDD 步骤：
//   1. 起真 Postgres + apply ai-svc 全套 migrations（i001..i008，含本 PR 新增 i008）
//   2. 断言：同 upload_id 二次 INSERT 抛 unique violation
//   3. 断言：upload_id=NULL → __legacy__ 回填后多行可入（partial unique 排除 legacy）
//   4. 断言：不同 upload_id 各自落一行
package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVoiceUploadId_DuplicateInsert_Rejected i008 后 voice_emotion_results.upload_id
// 唯一约束生效：同 upload_id 二次 INSERT 必抛 unique violation
func TestVoiceUploadId_DuplicateInsert_Rejected(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	// 1) 首次插入：upload_id = 'voice-uuid-001'
	require.NoError(t, db.WithContext(ctx).Exec(`
INSERT INTO emotion_echo_ai.voice_emotion_results
    (upload_id, user_id, primary_emotion, model)
VALUES ('voice-uuid-001', 1, 'calm', 'sensevoice-stub')`).Error)

	// 2) 第二次同 upload_id 期望 unique violation
	err := db.WithContext(ctx).Exec(`
INSERT INTO emotion_echo_ai.voice_emotion_results
    (upload_id, user_id, primary_emotion, model)
VALUES ('voice-uuid-001', 1, 'calm', 'sensevoice-stub')`).Error
	require.Error(t, err, "重复 upload_id 二次 INSERT 应被拒")
	assert.True(t,
		strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "duplicate"),
		"错误应含 unique / duplicate 关键字，实测: %v", err)

	// 3) 断言：表中只有 1 行
	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.voice_emotion_results WHERE upload_id = ?`,
			"voice-uuid-001").
		Scan(&count).Error)
	assert.Equal(t, int64(1), count, "同 upload_id 二次 INSERT 后表中仍应只有 1 行")
}

// TestVoiceUploadId_LegacyPlaceholder_AllowsMultipleNullRows i008 用 __legacy__
// 占位回填原 NULL 行，partial unique 索引 `WHERE upload_id <> '__legacy__'`
// 排除 legacy 行：多个 legacy 行可同时存在
func TestVoiceUploadId_LegacyPlaceholder_AllowsMultipleNullRows(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	// 1) 模拟历史 NULL 行（i008 回填后应全部变为 '__legacy__'）
	require.NoError(t, db.WithContext(ctx).Exec(`
INSERT INTO emotion_echo_ai.voice_emotion_results
    (upload_id, user_id, primary_emotion, model)
VALUES
    ('__legacy__', 1, 'calm', 'sensevoice-stub'),
    ('__legacy__', 2, 'happy', 'sensevoice-stub'),
    ('__legacy__', 3, 'sad', 'sensevoice-stub')`).Error)

	// 2) 多个 legacy 行同时存在（partial unique 不约束）
	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.voice_emotion_results WHERE upload_id = '__legacy__'`).
		Scan(&count).Error)
	assert.Equal(t, int64(3), count, "partial unique 应允许多个 __legacy__ 占位行")

	// 3) 真实 upload_id 仍唯一
	require.NoError(t, db.WithContext(ctx).Exec(`
INSERT INTO emotion_echo_ai.voice_emotion_results
    (upload_id, user_id, primary_emotion, model)
VALUES ('voice-uuid-002', 4, 'angry', 'sensevoice-stub')`).Error)

	require.Error(t, db.WithContext(ctx).Exec(`
INSERT INTO emotion_echo_ai.voice_emotion_results
    (upload_id, user_id, primary_emotion, model)
VALUES ('voice-uuid-002', 5, 'angry', 'sensevoice-stub')`).Error,
		"非 legacy upload_id 应被唯一约束拒")
}

// TestVoiceUploadId_DistinctIds_BothInserted 反例：不同 upload_id 各落一行
func TestVoiceUploadId_DistinctIds_BothInserted(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	for _, uid := range []string{"voice-uuid-100", "voice-uuid-200"} {
		require.NoError(t, db.WithContext(ctx).Exec(`
INSERT INTO emotion_echo_ai.voice_emotion_results
    (upload_id, user_id, primary_emotion, model)
VALUES (?, 1, 'calm', 'sensevoice-stub')`, uid).Error)
	}

	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.voice_emotion_results
		    WHERE upload_id IN ('voice-uuid-100', 'voice-uuid-200')`).
		Scan(&count).Error)
	assert.Equal(t, int64(2), count, "不同 upload_id 应各落 1 行")
}
