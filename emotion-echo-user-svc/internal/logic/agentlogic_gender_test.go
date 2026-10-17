// Package logic — agentlogic_gender_test.go
//
// 第二方核对（2026-10-17）抓出的**声明与行为不符**：`agentguard` 注释写着
// "gender / voice 是枚举（user-svc 侧按白名单校验）"，而 `validateAgentProfile` 只对
// **voice** 做了白名单 —— `gender` 只 `TrimSpace` 就落库（DDL 是 VARCHAR(8)，能塞下任意 8 字节）。
//
// 源计划 §4 的字段表明确要求：`gender` = 枚举 女/男/不指定，**非法值拒绝**。
// 本文件把该要求钉死（并让上面那句注释成为事实）。
package logic

import (
	"testing"

	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertAgentProfile_Gender_Whitelist(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"女", "男", "不指定", ""} {
		t.Run("allow/"+ok, func(t *testing.T) {
			svcCtx, _ := newAgentLogicEnv(t)
			req := validUpsert("阿哲")
			req.Gender = ok
			_, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(req)
			assert.NoError(t, err, "合法取值 %q 应当通过", ok)
		})
	}

	for _, bad := range []string{"male", "female", "其他", "男女", "UNKNOWN", "女性"} {
		t.Run("reject/"+bad, func(t *testing.T) {
			svcCtx, _ := newAgentLogicEnv(t)
			req := validUpsert("阿哲")
			req.Gender = bad
			_, err := NewUpsertAgentProfileLogic(agentCtx(1), svcCtx).UpsertAgentProfile(req)
			require.Error(t, err, "非法 gender %q 必须被拒（计划 §4：枚举校验）", bad)
		})
	}
}

// 性别是展示字段、**不进 prompt**，但落库值仍须受控（项目反复出现的"写入端用错值"类缺陷）
func TestUpsertAgentProfile_Gender_PersistedValueIsWhitelisted(t *testing.T) {
	t.Parallel()
	svcCtx, _ := newAgentLogicEnv(t)
	ctx := agentCtx(1)

	req := validUpsert("阿哲")
	req.Gender = "男"
	created, err := NewUpsertAgentProfileLogic(ctx, svcCtx).UpsertAgentProfile(req)
	require.NoError(t, err)
	assert.Equal(t, "男", created.Gender)

	list, err := NewListAgentProfilesLogic(ctx, svcCtx).ListAgentProfiles(&types.ListAgentProfilesReq{})
	require.NoError(t, err)
	for _, it := range list.Items {
		assert.Contains(t, []string{"女", "男", "不指定", ""}, it.Gender,
			"落库的 gender 必须落在白名单内（读了 %q）", it.Gender)
	}
}
