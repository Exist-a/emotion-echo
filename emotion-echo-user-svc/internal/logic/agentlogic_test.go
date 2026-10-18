// Package logic — agentlogic_test.go
//
// CL-06 Phase 2b（user-svc 服务层）：角色档案业务规则的契约测试（**先写测试**）。
//
// 本文件钉住的是**服务端强制**的规则（客户端绕不过）：
//
//	① 每户活跃角色 ≤5；② 字段上限（按**字符**计，不是字节）；③ voice 白名单；
//	④ 更新/删除前必须校验属主（防越权）；⑤ 默认「小E」**只在从未有过角色时**物化
//	（否则用户删光后再读会被重新物化 ⇒ 删除形同虚设）。
package logic

import (
	"context"
	"strings"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/middleware"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- 测试脚手架 ----------

func agentCtx(uid int64) context.Context {
	return context.WithValue(context.Background(), middleware.CtxUserIDKey{}, uid)
}

func newAgentLogicEnv(t *testing.T) (*svc.ServiceContext, repository.UserAgentProfileRepo) {
	t.Helper()
	repo := repository.NewInMemoryUserAgentProfileRepo()
	return svc.NewServiceContext(config.Config{}, nil, nil, repo), repo
}

func validUpsert(name string) *types.UpsertAgentProfileReq {
	return &types.UpsertAgentProfileReq{
		Name:       name,
		Gender:     "女",
		Persona:    "温柔、愿意倾听",
		Tone:       "轻快",
		Boundaries: []string{"不主动提工作"},
		Voice:      "FunAudioLLM/CosyVoice2-0.5B:bella",
	}
}

// ---------- ① 默认「小E」物化 ----------

func TestListAgentProfiles_FirstRead_SeedsDefaultXiaoE(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	resp, err := NewListAgentProfilesLogic(agentCtx(1), svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1, "首次读应当物化默认角色")
	got := resp.Items[0]
	assert.Equal(t, "小E", got.Name)
	assert.True(t, got.IsDefault, "默认角色必须带 is_default 标记（前端据此显示「默认」）")
	assert.Equal(t, DefaultAgentVoice, got.Voice)
	assert.NotZero(t, got.ID)
}

func TestListAgentProfiles_SecondRead_DoesNotSeedAgain(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	l := NewListAgentProfilesLogic(agentCtx(1), svcCtx)

	_, err := l.ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	resp, err := l.ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1, "第二次读不得再物化（否则每次读都多一个角色）")
}

func TestListAgentProfiles_AfterDeletingDefault_DoesNotReseed(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	listResp, err := NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	require.Len(t, listResp.Items, 1)

	_, err = NewDeleteAgentProfileLogic(ctx, svcCtx).DeleteAgentProfile(&types.DeleteAgentProfileReq{ID: listResp.Items[0].ID})
	require.NoError(t, err)

	resp, err := NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	assert.Empty(t, resp.Items, "用户主动删光后必须允许 0 角色（不得重新物化小E）")
}

func TestListAgentProfiles_Unauthorized_WhenNoUserID(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	_, err := NewListAgentProfilesLogic(context.Background(), svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// ---------- ② 每户 ≤5 ----------

func TestUpsertAgentProfile_RejectsSixthProfile(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	// 真实客户端会先加载列表（触发默认「小E」物化）再新建 ⇒ 测试照此走。
	// ⚠️ 作者注：本测试初版漏了这一步、误以为"物化发生在 Upsert 时"，导致**假失败**；
	// 物化的触发点是 **List**（见 ListAgentProfilesLogic），Upsert 不物化。
	seed, err := NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	require.Len(t, seed.Items, 1, "默认「小E」应已占 1 个名额")

	// 再建 4 个到上限（1 + 4 = 5）
	for i := 0; i < MaxAgentProfilesPerUser-1; i++ {
		_, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("角色" + string(rune('A'+i))))
		require.NoError(t, err, "第 %d 个应当通过", i+2)
	}

	_, err = NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("第六个"))
	assert.ErrorIs(t, err, ErrAgentLimitReached, "第 6 个必须被拒（默认角色计入上限）")

	list, err := NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	assert.Len(t, list.Items, MaxAgentProfilesPerUser)
}

func TestUpsertAgentProfile_AfterDelete_CanCreateAgain(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	var firstID int64
	for i := 0; i < MaxAgentProfilesPerUser; i++ {
		p, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("角色" + string(rune('A'+i))))
		require.NoError(t, err)
		if i == 0 {
			firstID = p.ID
		}
	}
	require.ErrorIs(t, func() error {
		_, e := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("超限"))
		return e
	}(), ErrAgentLimitReached)

	// 删一个 ⇒ 腾出名额（软删除不计入 active 上限）
	_, err := NewDeleteAgentProfileLogic(ctx, svcCtx).DeleteAgentProfile(&types.DeleteAgentProfileReq{ID: firstID})
	require.NoError(t, err)
	_, err = NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("补位"))
	assert.NoError(t, err, "删掉一个后应当能再建（上限按活跃数判定）")
}

// ---------- ③ 字段上限 / 白名单（按字符计） ----------

func TestUpsertAgentProfile_ValidatesFieldLimits(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	tests := []struct {
		name   string
		mutate func(r *types.UpsertAgentProfileReq)
	}{
		{"name 超 20 字", func(r *types.UpsertAgentProfileReq) { r.Name = strings.Repeat("名", 21) }},
		{"name 为空", func(r *types.UpsertAgentProfileReq) { r.Name = "   " }},
		{"persona 超 500 字", func(r *types.UpsertAgentProfileReq) { r.Persona = strings.Repeat("人", 501) }},
		{"tone 超 100 字", func(r *types.UpsertAgentProfileReq) { r.Tone = strings.Repeat("语", 101) }},
		{"boundaries 超 5 条", func(r *types.UpsertAgentProfileReq) {
			r.Boundaries = []string{"a", "b", "c", "d", "e", "f"}
		}},
		{"boundary 单条超 50 字", func(r *types.UpsertAgentProfileReq) {
			r.Boundaries = []string{strings.Repeat("边", 51)}
		}},
		{"voice 不在白名单", func(r *types.UpsertAgentProfileReq) { r.Voice = "FunAudioLLM/CosyVoice2-0.5B:eve" }},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := validUpsert("合法名")
			tc.mutate(req)
			_, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(req)
			assert.ErrorIs(t, err, ErrValidation, "非法输入必须被拒（**拒绝保存**，不是静默截断）")
		})
	}
}

func TestUpsertAgentProfile_EmptyVoice_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	req := validUpsert("没选音色")
	req.Voice = ""
	got, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(req)
	require.NoError(t, err)
	assert.Equal(t, DefaultAgentVoice, got.Voice, "空音色回落默认（前端可以不传）")
}

func TestUpsertAgentProfile_TrimsWhitespaceAndDropsEmptyBoundaries(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	req := validUpsert("  小暖  ")
	req.Tone = "  安静  "
	req.Boundaries = []string{"  不提工作  ", "", "   "}
	got, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(req)
	require.NoError(t, err)
	assert.Equal(t, "小暖", got.Name)
	assert.Equal(t, "安静", got.Tone)
	assert.Equal(t, []string{"不提工作"}, got.Boundaries, "空白项应被丢弃、首尾空白应被 trim")
}

func TestUpsertAgentProfile_AcceptsAllEightVoices(t *testing.T) {
	t.Parallel()
	for i, voice := range AllowedAgentVoiceList {
		i, voice := i, voice
		t.Run(voice, func(t *testing.T) {
			t.Parallel()
			svcCtx, _ := newAgentLogicEnv(t)
			req := validUpsert("角色" + string(rune('A'+i)))
			req.Voice = voice
			got, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(req)
			require.NoError(t, err)
			assert.Equal(t, voice, got.Voice)
		})
	}
}

// ---------- ④ 属主校验（IDOR） ----------

func TestUpsertAgentProfile_UpdateOtherUsersProfile_NotFound(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	other, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(validUpsert("小E"))
	require.NoError(t, err)

	req := validUpsert("越权改名")
	req.ID = other.ID
	_, err = NewUpsertAgentProfileLogic(agentCtx(2), svcCtx).UpsertAgentProfile(req)
	assert.ErrorIs(t, err, ErrAgentNotFound, "跨用户更新必须找不到（不得改到别人角色）")
}

func TestUpsertAgentProfile_UpdateMissing_NotFound(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	req := validUpsert("不存在")
	req.ID = 99999
	_, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(req)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestUpsertAgentProfile_Update_PersistsChanges(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	created, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(validUpsert("小E"))
	require.NoError(t, err)

	req := validUpsert("小暖")
	req.ID = created.ID
	req.IsDefault = true
	updated, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(req)
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID, "更新不得改 ID")
	assert.Equal(t, "小暖", updated.Name)
	assert.True(t, updated.IsDefault)
}

func TestDeleteAgentProfile_OtherUsersProfile_NotFound(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)

	other, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(validUpsert("小E"))
	require.NoError(t, err)

	_, err = NewDeleteAgentProfileLogic(agentCtx(2), svcCtx).DeleteAgentProfile(&types.DeleteAgentProfileReq{ID: other.ID})
	assert.ErrorIs(t, err, ErrAgentNotFound)

	// 所有者侧仍在
	list, err := NewListAgentProfilesLogic(agentCtx(1), svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	assert.Len(t, list.Items, 1)
}

func TestDeleteAgentProfile_Then_TwiceIsIdempotentError(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	list, err := NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	id := list.Items[0].ID

	_, err = NewDeleteAgentProfileLogic(ctx, svcCtx).DeleteAgentProfile(&types.DeleteAgentProfileReq{ID: id})
	require.NoError(t, err)

	// 第二次：已删 ⇒ 对外按"找不到"处理（HTTP 语义一致；仓储层本身幂等）
	_, err = NewDeleteAgentProfileLogic(ctx, svcCtx).DeleteAgentProfile(&types.DeleteAgentProfileReq{ID: id})
	assert.ErrorIs(t, err, ErrAgentNotFound)
}
