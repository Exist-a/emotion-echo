// Package logic — authlogic.go
//
// Stage 33 PR-19a：user-svc 真实 login / register 实现。
//
// 设计要点：
//   - 明文密码 → bcrypt.Hash 入库
//   - login 调 bcrypt.Verify 比对
//   - register 唯一性由 repo.UsernameExists 保证（DB UNIQUE 兜底）
//   - 不在这里实现限流 / 验证码校验（由 PR-19b BFF 侧负责）
package logic

import (
	"context"
	"errors"

	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/emotion-echo/shared/pkg/password"
	"github.com/zeromicro/go-zero/core/logx"
)

// AuthLogic 同时承载 Login / Register 流程；它们共享 ctx 与 svcCtx。
type AuthLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAuthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthLogic {
	return &AuthLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ErrInvalidCredentials 用户名不存在或密码错误（合并返回，避免用户名枚举）
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrUsernameTaken 用户名已存在
var ErrUsernameTaken = errors.New("username already taken")

// ErrValidation 入参校验失败
var ErrValidation = errors.New("validation failed")

// Login 用 username + password 校验并返回 UserInfo
//
// 错误语义：
//   - ErrValidation：username/password 为空
//   - ErrInvalidCredentials：用户不存在 OR 密码错误（合并）
//   - 其他 error：底层 DB / 哈希错误
func (l *AuthLogic) Login(req *types.LoginReq) (*types.LoginResp, error) {
	if req.Username == "" || req.Password == "" {
		return nil, ErrValidation
	}

	u, err := l.svcCtx.UserRepo.GetByUsername(l.ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		// 不暴露用户是否存在 → 统一返回 ErrInvalidCredentials
		return nil, ErrInvalidCredentials
	}
	if u.PasswordHash == nil || *u.PasswordHash == "" {
		// 账号无密码（历史遗留）→ 视为登录失败
		return nil, ErrInvalidCredentials
	}

	if !password.Verify(req.Password, *u.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	return &types.LoginResp{User: toUserInfo(u)}, nil
}

// Register 新建用户并 bcrypt 哈希密码
//
// 错误语义：
//   - ErrValidation：username/password 长度不合法
//   - ErrUsernameTaken：username 已存在
//   - 其他 error：底层 DB / 哈希错误
func (l *AuthLogic) Register(req *types.RegisterReq) (*types.RegisterResp, error) {
	if req.Username == "" || req.Password == "" {
		return nil, ErrValidation
	}
	// 最小长度校验（与 bcrypt 最小接受 1 字节对齐；这里取 6 字节防爆破）
	if len(req.Password) < 6 {
		return nil, ErrValidation
	}

	exists, err := l.svcCtx.UserRepo.UsernameExists(l.ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUsernameTaken
	}

	hash, err := password.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	u := &model.User{
		Username:     req.Username,
		PasswordHash: &hash,
		Phone:        req.Phone,
		Nickname:     req.Nickname,
	}
	if err := l.svcCtx.UserRepo.Create(l.ctx, u); err != nil {
		return nil, err
	}

	return &types.RegisterResp{User: toUserInfo(u)}, nil
}

// toUserInfo model.User → types.UserInfo（不暴露 PasswordHash）
func toUserInfo(u *model.User) types.UserInfo {
	phone := ""
	if u.Phone != nil {
		phone = *u.Phone
	}
	nick := ""
	if u.Nickname != nil {
		nick = *u.Nickname
	}
	return types.UserInfo{
		UserId:   u.ID,
		Account:  u.Username,
		Phone:    phone,
		Nickname: nick,
	}
}

// Compile-time guard that AuthLogic does not need repository.ErrNotFound directly.
var _ = repository.ErrNotFound
