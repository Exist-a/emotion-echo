// Package repository — user_agent_profile_repository_test.go
//
// CL-06 Phase 2（user-svc 角色档案）：仓储契约测试（红线先行 —— 先写测试，实现后补）。
//
// 覆盖：CRUD / 软删除（删后不出现在列表且 GetByID 返回 nil）/ **按 userID 隔离（防越权取他人角色）** /
// 计数（供服务层强制「每户 ≤５」）。
package repository

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newProfile(userID int64, name string) *model.UserAgentProfile {
	return &model.UserAgentProfile{
		UserID:     userID,
		Name:       name,
		Gender:     "女",
		Persona:    "温柔、愿意倾听",
		Tone:       "轻快",
		Voice:      "FunAudioLLM/CosyVoice2-0.5B:anna",
		Boundaries: []string{"不主动提工作"},
	}
}

func TestUserAgentProfileRepo_Create_AssignsIDAndReadsBack(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	created, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotZero(t, created.ID, "Create 必须回填自增 ID")

	got, err := repo.GetByID(ctx, 1, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "小E", got.Name)
	assert.Equal(t, []string{"不主动提工作"}, got.Boundaries, "boundaries 必须原样读回")
}

func TestUserAgentProfileRepo_ListByUserID_IsolatesUsers(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	_, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)
	_, err = repo.Create(ctx, newProfile(2, "别人的角色"))
	require.NoError(t, err)

	got, err := repo.ListByUserID(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 1, "不得返回他人角色")
	assert.Equal(t, "小E", got[0].Name)
}

func TestUserAgentProfileRepo_GetByID_WrongUser_ReturnsNil(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	created, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)

	// 同一 id 但换用户 → 必须取不到（防 IDOR）
	got, err := repo.GetByID(ctx, 2, created.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "跨用户按 id 取角色必须返回 nil")
}

func TestUserAgentProfileRepo_SoftDelete_HidesFromListAndGet(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	created, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, 1, created.ID))

	list, err := repo.ListByUserID(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, list, "软删除后不得出现在列表")

	got, err := repo.GetByID(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "软删除后 GetByID 必须返回 nil")
}

func TestUserAgentProfileRepo_SoftDelete_RepeatedIsNoop(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	created, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, 1, created.ID))
	require.NoError(t, repo.SoftDelete(ctx, 1, created.ID), "重复软删除必须幂等（不报错）")
}

func TestUserAgentProfileRepo_SoftDelete_WrongUser_DoesNotAffect(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	created, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)

	// 别人来删 → 不报错但不生效
	require.NoError(t, repo.SoftDelete(ctx, 2, created.ID))

	got, err := repo.GetByID(ctx, 1, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got, "跨用户软删除不得影响所有者数据")
}

func TestUserAgentProfileRepo_CountActive_CountsOnlyLive(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	var ids []int64
	for _, n := range []string{"a", "b", "c"} {
		p, err := repo.Create(ctx, newProfile(1, n))
		require.NoError(t, err)
		ids = append(ids, p.ID)
	}
	_, err := repo.Create(ctx, newProfile(2, "他人"))
	require.NoError(t, err)

	n, err := repo.CountActive(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	require.NoError(t, repo.SoftDelete(ctx, 1, ids[0]))
	n, err = repo.CountActive(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "软删除的不计入（≤５ 上限据此判定）")
}

func TestUserAgentProfileRepo_Update_OnlyOwnerAndPersistsFields(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryUserAgentProfileRepo()
	ctx := context.Background()

	created, err := repo.Create(ctx, newProfile(1, "小E"))
	require.NoError(t, err)

	created.Name = "小暖"
	created.Tone = "安静"
	created.Voice = "FunAudioLLM/CosyVoice2-0.5B:bella"
	created.IsDefault = true
	require.NoError(t, repo.Update(ctx, created))

	got, err := repo.GetByID(ctx, 1, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "小暖", got.Name)
	assert.Equal(t, "安静", got.Tone)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:bella", got.Voice)
	assert.True(t, got.IsDefault)

	// 换用户改同 id → 取不到（防止 Update 越权）
	other := *created
	other.UserID = 2
	other.Name = "越权改名"
	require.NoError(t, repo.Update(ctx, &other))
	got, err = repo.GetByID(ctx, 1, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "小暖", got.Name, "跨用户 Update 不得改动所有者数据")
}
