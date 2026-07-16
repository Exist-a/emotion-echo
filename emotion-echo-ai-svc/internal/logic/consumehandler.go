// Package logic 实现 ai-svc 的业务逻辑
//
// MessageCreatedHandler 是 ai-svc 的核心异步处理器：
//   - 消费 chat-svc 的 message.created 事件
//   - 用 analyzer 跑情绪分析
//   - 写 emotion_analysis 表
package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/zeromicro/go-zero/core/logx"
)

// MessageCreatedHandler 处理 message.created 事件
//
// 业务规则：
//   - 只分析 user 角色的消息（assistant / system 的跳过）
//   - event type 不匹配 → 跳过（无 error）
//   - data 解析失败 → 返回 error（sarama 会重试，最终入 DLQ）
type MessageCreatedHandler struct {
	logx.Logger
	repo     repository.EmotionRepo
	analyzer analyzer.Analyzer
}

// NewMessageCreatedHandler 构造 handler
func NewMessageCreatedHandler(repo repository.EmotionRepo, a analyzer.Analyzer) *MessageCreatedHandler {
	return &MessageCreatedHandler{
		Logger:   logx.WithContext(context.Background()),
		repo:     repo,
		analyzer: a,
	}
}

// Handle 处理一条事件
//
// 返回 nil → 提交 offset；返回 error → 重试
func (h *MessageCreatedHandler) Handle(ctx context.Context, evt *events.Event) error {
	// 1. 类型过滤
	if evt.Type != events.EventTypeMessageCreated {
		h.Debugf("skip event type=%s", evt.Type)
		return nil
	}

	// 2. 解析 data
	rawData, err := json.Marshal(evt.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	var data events.MessageCreatedData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}

	// 3. 只分析 user 消息
	if data.Role != "user" {
		h.Debugf("skip non-user role=%s messageID=%d", data.Role, data.MessageID)
		return nil
	}

	// 4. 跑分析
	result, err := h.analyzer.Analyze(ctx, data.Content)
	if err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	if result == nil {
		return errors.New("analyzer returned nil result")
	}

	// 5. 写库
	row := &model.EmotionAnalysis{
		MessageID:      data.MessageID,
		UserID:         data.UserID,
		ConversationID: data.ConversationID,
		PrimaryEmotion: result.PrimaryEmotion,
		SentimentScore: result.SentimentScore,
		Confidence:     result.Confidence,
		Model:          result.Model,
	}
	if err := h.repo.Create(ctx, row); err != nil {
		return fmt.Errorf("create emotion_analysis: %w", err)
	}

	h.Infof("analyzed messageID=%d emotion=%s score=%.2f",
		data.MessageID, result.PrimaryEmotion, result.SentimentScore)
	return nil
}