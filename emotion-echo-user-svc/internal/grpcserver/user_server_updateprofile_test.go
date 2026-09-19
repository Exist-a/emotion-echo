// Package grpcserver — user_server_updateprofile_test.go
//
// E2E-11：gRPC 的 UpdateProfile 必须把 proto 的 avatar_url 落到 types.UpdateProfileReq。
//
// 背景（与 E2E-F-72「端点已补 ≠ 功能可用」同源）：
//   - proto user.proto 定义 `optional string avatar_url = 2`；
//   - user-svc logic/updateprofilelogic.go 与 repository 都已正确处理 AvatarURL；
//   - 但 grpcserver/user_server.go 的 UpdateProfile 只组装 Nickname + Gender，
//     **avatar_url 在 proto → types 的转换里被丢掉** ⇒ 头像上传接口返 200，
//     数据库 avatar_url 仍为 NULL。
//
// dev 实测（2026-09-19）：BFF 侧修好透传后重跑，Playwright 断言 200 通过，
// 但 `SELECT avatar_url FROM users WHERE id=1` 仍为 NULL —— 说明链路还有第二处丢弃点。
package grpcserver

import (
	"context"
	"strconv"
	"testing"

	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// ctxWithUserID 把 user id 塞进 x-user-id metadata（与 BFF 真实调用一致）
func ctxWithUserID(uid int64) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(),
		"x-user-id", strconv.FormatInt(uid, 10))
}

// recAvatar 从 repo 读回当前 avatar_url（nil → 空串）
func recAvatar(t *testing.T, repo repository.UserRepo, uid int64) string {
	t.Helper()
	u, err := repo.GetByID(context.Background(), uid)
	require.NoError(t, err)
	require.NotNil(t, u)
	if u.AvatarURL == nil {
		return ""
	}
	return *u.AvatarURL
}

// TestUserServer_UpdateProfile_PersistsAvatarURL 钉住 proto → types → repo 的完整透传。
func TestUserServer_UpdateProfile_PersistsAvatarURL(t *testing.T) {
	repo := repository.NewInMemoryUserRepo()
	svcCtx := &svc.ServiceContext{UserRepo: repo}

	conn, cleanup := startUserTestServerWithCtx(t, svcCtx)
	defer cleanup()
	client := emotionuser.NewUserServiceClient(conn)

	// 前置：注册一个用户拿到 id
	regResp, err := client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "avatar_user",
		Password: "Password123",
	})
	require.NoError(t, err)
	uid := regResp.GetUser().GetId()
	require.NotZero(t, uid)

	// 带 x-user-id 调 UpdateProfile（与 BFF 真实调用一致）
	avatar := "http://localhost:9000/emotion-echo/avatars/1-abc123.png"
	updResp, err := client.UpdateProfile(ctxWithUserID(uid), &emotionuser.UpdateProfileRequest{
		AvatarUrl: &avatar,
	})
	require.NoError(t, err)

	assert.Equal(t, avatar, updResp.GetAvatarUrl(),
		"E2E-11: UpdateProfile 响应应回带 avatar_url")
	assert.Equal(t, avatar, recAvatar(t, repo, uid),
		"E2E-11: avatar_url 必须落到 repo。原实现 proto→types 转换只取 Nickname+Gender，"+
			"avatar_url 被丢弃 ⇒ 头像上传返 200 但数据库恒 NULL。")
}

// Nil AvatarURL 不应清空已有值（partial update 语义）
func TestUserServer_UpdateProfile_NilAvatarURL_KeepsExisting(t *testing.T) {
	repo := repository.NewInMemoryUserRepo()
	svcCtx := &svc.ServiceContext{UserRepo: repo}

	conn, cleanup := startUserTestServerWithCtx(t, svcCtx)
	defer cleanup()
	client := emotionuser.NewUserServiceClient(conn)

	regResp, err := client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "keep_avatar_user",
		Password: "Password123",
	})
	require.NoError(t, err)
	uid := regResp.GetUser().GetId()

	avatar := "http://localhost:9000/emotion-echo/avatars/keep.png"
	_, err = client.UpdateProfile(ctxWithUserID(uid), &emotionuser.UpdateProfileRequest{
		AvatarUrl: &avatar,
	})
	require.NoError(t, err)

	// 只改昵称，不传 avatar
	nick := "改个昵称"
	_, err = client.UpdateProfile(ctxWithUserID(uid), &emotionuser.UpdateProfileRequest{
		Nickname: &nick,
	})
	require.NoError(t, err)

	assert.Equal(t, avatar, recAvatar(t, repo, uid),
		"E2E-11: 不传 avatar_url 时不应清空既有头像（partial update 语义）")
}
