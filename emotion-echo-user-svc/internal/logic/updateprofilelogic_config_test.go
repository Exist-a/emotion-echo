package logic

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-12 RED：UpdateProfileLogic 必须透传 config 到 repository。
// 当前 types.UpdateProfileReq 无 Config 字段，此测试将编译失败（RED）。
func TestUpdateProfileLogic_WithConfig_PersistsAndReturns(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "echo",
		Nickname: sp("Echo"),
	}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	config := map[string]any{"fontSize": "18px", "theme": "dark"}
	resp, err := l.UpdateProfile(&types.UpdateProfileReq{
		Config: &config,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.User.Config, "响应 UserInfo 必须包含 Config")
	assert.Equal(t, "18px", resp.User.Config["fontSize"])
	assert.Equal(t, "dark", resp.User.Config["theme"])
}

// E2E-12 RED：只改 config 不动其他字段。
func TestUpdateProfileLogic_OnlyConfig_DoesNotTouchOtherFields(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "echo",
		Nickname: sp("Original"),
	}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	config := map[string]any{"theme": "auto"}
	resp, err := l.UpdateProfile(&types.UpdateProfileReq{
		Config: &config,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Original", resp.User.Nickname, "nickname 不应被 config 更新影响")
}