// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"

	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListEmotionByConversationLogic 处理 GET /api/v1/emotion/conversation/:conversationId
type ListEmotionByConversationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmotionByConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmotionByConversationLogic {
	return &ListEmotionByConversationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListEmotionByConversation 列出会话下所有情绪分析
//
// 返回的 slice 保证非 nil（即使没有数据也是 []EmotionView{}）
// 业务可安全遍历或 JSON 序列化
func (l *ListEmotionByConversationLogic) ListEmotionByConversation(req *types.ListEmotionByConversationReq) (resp *types.ListEmotionByConversationResp, err error) {
	rows, err := l.svcCtx.EmotionRepo.ListByConversationID(l.ctx, req.ConversationId)
	if err != nil {
		return nil, err
	}

	emotions := make([]types.EmotionView, 0, len(rows))
	for _, e := range rows {
		emotions = append(emotions, types.EmotionView{
			Id:             e.ID,
			MessageId:      e.MessageID,
			ConversationId: e.ConversationID,
			UserId:         e.UserID,
			PrimaryEmotion: e.PrimaryEmotion,
			SentimentScore: e.SentimentScore,
			Confidence:     e.Confidence,
			Model:          e.Model,
			CreatedAt:      e.CreatedAt.UnixMilli(),
		})
	}

	return &types.ListEmotionByConversationResp{Emotions: emotions}, nil
}