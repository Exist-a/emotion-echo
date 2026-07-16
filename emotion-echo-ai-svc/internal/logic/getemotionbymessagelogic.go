// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"errors"

	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetEmotionByMessageLogic 处理 GET /api/v1/emotion/message/:messageId
type GetEmotionByMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmotionByMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmotionByMessageLogic {
	return &GetEmotionByMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetEmotionByMessage 按 messageID 查情绪分析
func (l *GetEmotionByMessageLogic) GetEmotionByMessage(req *types.GetEmotionByMessageReq) (resp *types.GetEmotionByMessageResp, err error) {
	e, err := l.svcCtx.EmotionRepo.GetByMessageID(l.ctx, req.MessageId)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errors.New("not found: no emotion analysis for this message")
	}
	return &types.GetEmotionByMessageResp{
		Emotion: types.EmotionView{
			Id:             e.ID,
			MessageId:      e.MessageID,
			ConversationId: e.ConversationID,
			UserId:         e.UserID,
			PrimaryEmotion: e.PrimaryEmotion,
			SentimentScore: e.SentimentScore,
			Confidence:     e.Confidence,
			Model:          e.Model,
			CreatedAt:      e.CreatedAt.UnixMilli(),
		},
	}, nil
}