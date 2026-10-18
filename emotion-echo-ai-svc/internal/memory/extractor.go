// Package memory 实现 CL-08a 的用户事实抽取。
//
// 边界（本切片）：
//   - 只做「从一条 user 消息里抽结构事实」，不做实体消解、不做图、不做向量召回。
//   - 抽取是**异步增强**：失败只让消费重试/DLQ，绝不进入发消息关键路径（CL-D-33）。
package memory

import (
	"context"
	"time"

	"emotion-echo-ai-svc/internal/model"
)

// ExtractInput 一次抽取的输入（一条 user 消息或一条自述条）。
type ExtractInput struct {
	UserID         int64
	ConversationID int64
	// MessageID 兼容字段：message 来源时等于消息 id（自述条为 0）
	MessageID int64
	// SourceKind / SourceID 来源身份（message / note）；SourceKind 空 = message
	SourceKind string
	SourceID   int64
	Content    string
	// OccurredAt 来源时间；LLM 未给出 valid_from 时回落到它。
	OccurredAt time.Time
}

// Extractor 事实抽取接口。
//
// 单测注入确定性假实现（不调真 LLM）；生产实现见 LLMExtractor。
type Extractor interface {
	Extract(ctx context.Context, in ExtractInput) ([]model.MemoryFact, error)
}
