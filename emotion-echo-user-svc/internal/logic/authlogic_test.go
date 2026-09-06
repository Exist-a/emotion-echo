// Package logic — authlogic_test.go
//
// Stage 33 PR-19a：user-svc 真实 login / register 的 TDD 测试。
package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/emotion-echo/shared/pkg/password"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAuthLogic(repo repository.UserRepo) *AuthLogic {
	return NewAuthLogic(context.Background(), &svc.ServiceContext{UserRepo: repo})
}

// =============================================================================
// Login tests
// =============================================================================

func TestAuthLogic_Login_CorrectPassword_ReturnsUserInfo(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	hash, err := password.Hash("correct-password")
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:           100,
		Username:     "alice",
		PasswordHash: &hash,
	}))

	l := newTestAuthLogic(repo)
	resp, err := l.Login(&types.LoginReq{Username: "alice", Password: "correct-password"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(100), resp.User.UserId)
	assert.Equal(t, "alice", resp.User.Account)
}

func TestAuthLogic_Login_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	hash, _ := password.Hash("correct-password")
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID: 1, Username: "alice", PasswordHash: &hash,
	}))

	l := newTestAuthLogic(repo)
	resp, err := l.Login(&types.LoginReq{Username: "alice", Password: "wrong-password"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))
}

func TestAuthLogic_Login_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)
	resp, err := l.Login(&types.LoginReq{Username: "ghost", Password: "any"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))
}

func TestAuthLogic_Login_EmptyFields_ReturnsValidation(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	// 空 username
	resp, err := l.Login(&types.LoginReq{Username: "", Password: "x"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation))

	// 空 password
	resp, err = l.Login(&types.LoginReq{Username: "alice", Password: ""})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation))
}

func TestAuthLogic_Login_NoPasswordHash_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()

	// 模拟历史遗留账号：存在 user 但 password_hash 为空（早期 mock 注册）
	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID: 1, Username: "legacy", PasswordHash: nil,
	}))

	l := newTestAuthLogic(repo)
	resp, err := l.Login(&types.LoginReq{Username: "legacy", Password: "anything"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCredentials))
}

// =============================================================================
// Register tests
// =============================================================================

func TestAuthLogic_Register_Success_StoresBcryptHash(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	resp, err := l.Register(&types.RegisterReq{
		Username: "newuser",
		Password: "valid-password",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "newuser", resp.User.Account)
	assert.NotZero(t, resp.User.UserId)

	// 验证入库的是 bcrypt 哈希（非明文），且 Verify 能匹配
	stored, err := repo.GetByUsername(context.Background(), "newuser")
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.NotNil(t, stored.PasswordHash)
	assert.NotEqual(t, "valid-password", *stored.PasswordHash)
	assert.True(t, password.Verify("valid-password", *stored.PasswordHash))
	assert.Len(t, *stored.PasswordHash, 60)
}

func TestAuthLogic_Register_DuplicateUsername_ReturnsUsernameTaken(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	hash, _ := password.Hash("any")
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID: 1, Username: "alice", PasswordHash: &hash,
	}))

	l := newTestAuthLogic(repo)
	resp, err := l.Register(&types.RegisterReq{
		Username: "alice",
		Password: "valid-password",
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUsernameTaken))
}

func TestAuthLogic_Register_EmptyFields_ReturnsValidation(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	// 空 username
	_, err := l.Register(&types.RegisterReq{Username: "", Password: "valid-password"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation))

	// 空 password
	_, err = l.Register(&types.RegisterReq{Username: "newuser", Password: ""})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation))
}

func TestAuthLogic_Register_TooShortPassword_ReturnsValidation(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	// 5 字符（< 6 字符最小长度）
	_, err := l.Register(&types.RegisterReq{Username: "newuser", Password: "12345"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation))
}

func TestAuthLogic_Register_WithPhoneAndNickname_PersistsOptionalFields(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	phone := "13800000001"
	nick := "Nick"
	resp, err := l.Register(&types.RegisterReq{
		Username: "phoneuser",
		Password: "valid-password",
		Phone:    &phone,
		Nickname: &nick,
	})
	require.NoError(t, err)
	assert.Equal(t, phone, resp.User.Phone)
	assert.Equal(t, nick, resp.User.Nickname)

	stored, _ := repo.GetByUsername(context.Background(), "phoneuser")
	require.NotNil(t, stored)
	require.NotNil(t, stored.Phone)
	assert.Equal(t, phone, *stored.Phone)
}

// =============================================================================
// Sprint 1 PR-4c-3: ResetPassword tests
// =============================================================================

func TestAuthLogic_ResetPassword_Success(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	// 先注册一个用户
	_, err := l.Register(&types.RegisterReq{Username: "alice", Password: "old-password"})
	require.NoError(t, err)

	// reset password
	resp, err := l.ResetPassword(&types.ResetPasswordReq{
		Username:         "alice",
		VerificationCode: "123456",
		NewPassword:      "new-password-789",
	})
	require.NoError(t, err)
	assert.Equal(t, "alice", resp.User.Account)

	// 验证新密码可用 Login 登录
	login, err := l.Login(&types.LoginReq{Username: "alice", Password: "new-password-789"})
	require.NoError(t, err, "新密码应可登录")
	assert.Equal(t, "alice", login.User.Account)
}

func TestAuthLogic_ResetPassword_OldPasswordNoLongerValid(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	_, err := l.Register(&types.RegisterReq{Username: "bob", Password: "old-password"})
	require.NoError(t, err)

	_, err = l.ResetPassword(&types.ResetPasswordReq{
		Username:         "bob",
		VerificationCode: "123456",
		NewPassword:      "new-password-789",
	})
	require.NoError(t, err)

	// 旧密码应不可登录
	_, err = l.Login(&types.LoginReq{Username: "bob", Password: "old-password"})
	assert.ErrorIs(t, err, ErrInvalidCredentials, "旧密码应失效")
}

func TestAuthLogic_ResetPassword_EmptyFields_ReturnsValidation(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	_, err := l.ResetPassword(&types.ResetPasswordReq{Username: "", NewPassword: "x", VerificationCode: "1"})
	assert.ErrorIs(t, err, ErrValidation)

	_, err = l.ResetPassword(&types.ResetPasswordReq{Username: "u", NewPassword: "", VerificationCode: "1"})
	assert.ErrorIs(t, err, ErrValidation)
}

func TestAuthLogic_ResetPassword_ShortPassword_ReturnsValidation(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	_, err := l.ResetPassword(&types.ResetPasswordReq{Username: "u", NewPassword: "abc", VerificationCode: "1"})
	assert.ErrorIs(t, err, ErrValidation, "< 6 字节密码应返 ErrValidation")
}

func TestAuthLogic_ResetPassword_EmptyVerifyCode_ReturnsInvalidVerifyCode(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)
	_, err := l.Register(&types.RegisterReq{Username: "carol", Password: "old-password"})
	require.NoError(t, err)

	_, err = l.ResetPassword(&types.ResetPasswordReq{
		Username:         "carol",
		VerificationCode: "",
		NewPassword:      "new-password",
	})
	assert.ErrorIs(t, err, ErrInvalidVerifyCode)
}

func TestAuthLogic_ResetPassword_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := newTestAuthLogic(repo)

	_, err := l.ResetPassword(&types.ResetPasswordReq{
		Username:         "nonexistent",
		VerificationCode: "1",
		NewPassword:      "new-password-789",
	})
	assert.ErrorIs(t, err, ErrInvalidCredentials, "不存在的用户名应合并返 ErrInvalidCredentials 防枚举")
}
