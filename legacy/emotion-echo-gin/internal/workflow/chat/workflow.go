// Package chat 提供AI对话情绪分析工作流
//
// 工作流节点：
//   emotion_analysis:  调用LLM分析用户消息情绪
//   prompt_selector:   根据情绪选择对应的系统提示词
//
// 使用场景：
//   在 AIService.StreamChat 中同步执行，根据用户消息动态选择AI回复策略。
//   执行时间约200-500ms，失败时降级使用默认提示词。
package chat

import (
	"context"

	"emotion-echo-gin/internal/workflow/graph"
	"emotion-echo-gin/internal/workflow/text"
)

// BuildEmotionWorkflow 构建情绪分析工作流
//
// 参数 llmCaller: 调用大模型的函数，用于情绪分析
// 返回值: 包含2个节点的DAG（情绪分析 → Prompt选择）
func BuildEmotionWorkflow(
	llmCaller func(ctx context.Context, prompt string) (string, error),
) *graph.Graph {
	return text.NewOnlineWorkflow(llmCaller)
}
