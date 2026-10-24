//go:build integration
// +build integration

// memory_episode_entity_repo_integration_test.go —— CL-08c Phase 1：
// 情节 / 实体仓储的**真库**语义（内存替身之外，PG 实现必须被真库钉死）。
//
// 覆盖：
//  1. T-2 写入读回一致（真 PG 往返，字段逐一比对）；
//  2. 幂等（uq_memory_episode_source：同来源重复 upsert ⇒ 1 行且不覆盖）；
//  3. §契约 5（schema 与写入端一致性）：每个 CHECK 枚举列都被真库拦
//     —— status / source_kind 写入非法值必须报错（抓"DDL 对了但写入用错值"）；
//  4. 实体唯一锚点：同 (user_id, name) 一行 + 摘要刷新 + 软删复活 + 跨用户独立。

package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkMemEpisode(userID, convID, sourceID int64, name, content string, validAt time.Time) *model.MemoryEpisode {
	return &model.MemoryEpisode{
		UserID:         userID,
		ConversationID: convID,
		SourceKind:     model.MemorySourceKindMessage,
		SourceID:       sourceID,
		Name:           name,
		Content:        content,
		ValidAt:        validAt,
	}
}

func TestPostgresMemoryGraphRepo_UpsertEpisode_RoundTripAndIdempotent(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryGraphRepo(db)
	validAt := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)

	ep := mkMemEpisode(1, 7, 100, "msg-100", "我最近搬到了上海工作", validAt)
	created, err := repo.UpsertEpisode(ctx, ep)
	require.NoError(t, err)
	require.True(t, created, "首次写入必须 created=true")
	require.NotZero(t, ep.ID, "id 必须由库生成并回填")

	// T-2：写入读回一致
	got, err := repo.GetEpisode(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(1), got.UserID)
	assert.Equal(t, int64(7), got.ConversationID)
	assert.Equal(t, "msg-100", got.Name)
	assert.Equal(t, "我最近搬到了上海工作", got.Content)
	assert.True(t, got.ValidAt.UTC().Equal(validAt), "valid_at 必须原样读回")
	assert.Equal(t, model.MemoryNodeStatusActive, got.Status)
	assert.Nil(t, got.DeletedAt)

	// 幂等：同来源重复投递不重复、不覆盖
	second, err := repo.UpsertEpisode(ctx, mkMemEpisode(1, 7, 100, "msg-100", "重放文本", validAt))
	require.NoError(t, err)
	assert.False(t, second, "重复来源必须幂等跳过")

	rows, err := repo.ListEpisodesByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1, "同来源重复 upsert 只允许一行")
	assert.Equal(t, "我最近搬到了上海工作", rows[0].Content, "重复 upsert 不得覆盖首次内容")
}

// §契约 5：CHECK 枚举列的真库拦截 —— 写入端用错值必须当场报错，
// 而不是静默落进一个"DDL 以为对"的值。
func TestPostgresMemoryGraphRepo_CheckConstraints_RejectBadEnumValues(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()

	// memory_episode.status 非法值（写入端常量只有 active/deleted）
	err := db.WithContext(ctx).Exec(
		`INSERT INTO emotion_echo_ai.memory_episode
		    (user_id, conversation_id, source_kind, source_id, name, content, valid_at, status)
		 VALUES (1, 7, 'message', 900, 'n', 'c', NOW(), 'bogus')`).Error
	require.Error(t, err, "memory_episode.status 非法值必须被 CHECK 拦下")
	assert.True(t, strings.Contains(err.Error(), "chk_memory_episode_status"),
		"报错必须来自 CHECK 约束（实测 got: %v）", err)

	// memory_episode.source_kind 非法值（写入端常量只有 message/note）
	err = db.WithContext(ctx).Exec(
		`INSERT INTO emotion_echo_ai.memory_episode
		    (user_id, conversation_id, source_kind, source_id, name, content, valid_at)
		 VALUES (1, 7, 'conversation', 901, 'n', 'c', NOW())`).Error
	require.Error(t, err, "memory_episode.source_kind 非法值必须被 CHECK 拦下")
	assert.True(t, strings.Contains(err.Error(), "chk_memory_episode_source_kind"))

	// memory_entity.status 非法值
	err = db.WithContext(ctx).Exec(
		`INSERT INTO emotion_echo_ai.memory_entity (user_id, name, summary, status)
		 VALUES (1, '母亲', 's', 'superseded')`).Error
	require.Error(t, err, "memory_entity.status 非法值必须被 CHECK 拦下")

	// 合法值对照：同样两条用合法值必须能写入（守卫不是把整张表锁死）
	require.NoError(t, db.WithContext(ctx).Exec(
		`INSERT INTO emotion_echo_ai.memory_episode
		    (user_id, conversation_id, source_kind, source_id, name, content, valid_at)
		 VALUES (1, 7, 'note', 902, 'note-902', 'c', NOW())`).Error,
		"合法值（note）必须可写入")
}

func TestPostgresMemoryGraphRepo_UpsertEntity_UniqueAnchorAndRevive(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryGraphRepo(db)

	created, err := repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 1, Name: "母亲", Summary: "用户的母亲"})
	require.NoError(t, err)
	require.True(t, created)

	// 同名重复 upsert ⇒ 刷新摘要、不新增行
	created, err = repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 1, Name: "母亲", Summary: "用户母亲，上周住院"})
	require.NoError(t, err)
	assert.False(t, created)

	got, err := repo.GetEntityByName(ctx, 1, "母亲")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "用户母亲，上周住院", got.Summary, "摘要必须刷新为最新值")

	// 跨用户同名 ⇒ 独立行（记忆绝不共享）
	_, err = repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 2, Name: "母亲", Summary: "另一人的母亲"})
	require.NoError(t, err)
	other, err := repo.GetEntityByName(ctx, 2, "母亲")
	require.NoError(t, err)
	require.NotNil(t, other)
	assert.NotEqual(t, got.ID, other.ID, "跨用户同名实体必须是独立行")

	var cnt int64
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_ai.memory_entity WHERE user_id = 1 AND name = '母亲'`).Scan(&cnt).Error)
	assert.Equal(t, int64(1), cnt, "同 (user_id, name) 只允许一行")

	// 软删后再抽取 ⇒ 复活（新证据应生效，不新建行）
	require.NoError(t, db.WithContext(ctx).Exec(
		`UPDATE emotion_echo_ai.memory_entity SET status = 'deleted', deleted_at = NOW()
		  WHERE user_id = 1 AND name = '母亲'`).Error)
	gone, err := repo.GetEntityByName(ctx, 1, "母亲")
	require.NoError(t, err)
	assert.Nil(t, gone, "软删后按名查询不可见")

	created, err = repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 1, Name: "母亲", Summary: "重新提起"})
	require.NoError(t, err)
	assert.True(t, created, "复活视为一次新落行")

	revived, err := repo.GetEntityByName(ctx, 1, "母亲")
	require.NoError(t, err)
	require.NotNil(t, revived)
	assert.Equal(t, model.MemoryNodeStatusActive, revived.Status)
	assert.Nil(t, revived.DeletedAt, "复活必须清掉 deleted_at")

	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_ai.memory_entity WHERE user_id = 1 AND name = '母亲'`).Scan(&cnt).Error)
	assert.Equal(t, int64(1), cnt, "复活不得新建行")
}

func TestPostgresMemoryGraphRepo_UpsertEpisode_RejectsInvalidInput(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryGraphRepo(db)

	_, err := repo.UpsertEpisode(ctx, mkMemEpisode(1, 7, 100, "", "内容", time.Now().UTC()))
	require.Error(t, err, "空 name 必须被仓储层拦下（不落半行）")
	assert.ErrorIs(t, err, repository.ErrInvalidMemoryGraph)

	_, err = repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 0, Name: "母亲"})
	require.Error(t, err, "user_id=0 必须被仓储层拦下")
	assert.ErrorIs(t, err, repository.ErrInvalidMemoryGraph)
}
