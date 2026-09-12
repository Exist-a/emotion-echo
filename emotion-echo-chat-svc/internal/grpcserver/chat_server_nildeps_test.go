// Package grpcserver — chat_server_nildeps_test.go
//
// Stage 77 RED：与 user-svc 同构的 nil-repo 契约（stage-76 §二.3）。
// openPostgres 失败时 main.go 带 ConversationRepo=nil 的 ServiceContext 启动，
// gRPC 端点必须返 codes.Unavailable，绝不 panic。
package grpcserver

import (
	"context"
	"testing"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"emotion-echo-chat-svc/internal/svc"
)

func TestChatServer_CreateConversation_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &chatServer{svcCtx: &svc.ServiceContext{}}
	_, err := s.CreateConversation(context.Background(), &emotionchat.CreateConversationRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "repo 未注入应返 Unavailable 而非 panic")
}

func TestChatServer_ListConversations_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &chatServer{svcCtx: &svc.ServiceContext{}}
	_, err := s.ListConversations(context.Background(), &emotionchat.ListConversationsRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}
