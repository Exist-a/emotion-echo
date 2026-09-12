// Package grpcserver — server_nildeps_test.go
//
// Stage 77 RED：与 user-svc 同构的 nil-repo 契约（stage-76 §二.3）。
// openPostgres 失败时 main.go 传 repo=nil 启动（既有先例：fusedEmotionRepo nil
// → Unimplemented），EmotionRepo nil 时查询端点必须返 codes.Unavailable，绝不 panic。
package grpcserver

import (
	"context"
	"testing"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEmotionQueryServer_GetEmotionByMessage_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &emotionQueryServer{}
	_, err := s.GetEmotionByMessage(context.Background(), &emotionquery.GetEmotionByMessageRequest{
		MessageId: 1,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "repo 未注入应返 Unavailable 而非 panic")
}

func TestEmotionQueryServer_GetEmotionByConversation_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &emotionQueryServer{}
	_, err := s.GetEmotionByConversation(context.Background(), &emotionquery.GetEmotionByConversationRequest{
		ConversationId: 1,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}
