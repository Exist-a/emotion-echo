// Package react 提供 ReAct (Reasoning + Acting) 模式实现
//
// ReAct 循环：
//   Thought（思考）→ Action（行动）→ Observation（观察）→ ... → Final Answer
//
// 适用场景：
//   - 需要多轮推理的复杂分析
//   - 需要查询外部信息的决策
//   - 需要验证和修正的结论
package react

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"emotion-echo-gin/internal/workflow/graph"
	"emotion-echo-gin/internal/workflow/tools"
)

// Loop 执行 ReAct 循环
// 从 state 中读取 "react_messages"（对话历史），执行多轮反思
func Loop(
	ctx context.Context,
	llmCaller func(ctx context.Context, prompt string) (string, error),
	toolRegistry *tools.Registry,
	maxIterations int,
	state graph.State,
) (graph.State, error) {
	if maxIterations <= 0 {
		maxIterations = 5
	}

	for i := 0; i < maxIterations; i++ {
		// Step 1: Thought
		thoughtPrompt := buildThoughtPrompt(state)
		thoughtResponse, err := llmCaller(ctx, thoughtPrompt)
		if err != nil {
			state.Set("react_error", fmt.Sprintf("thought failed: %v", err))
			return state, nil
		}

		thought := parseThought(thoughtResponse)
		state.Set("react_thought", thought.Thought)
		state.Set("react_iteration", i+1)

		// 检查是否已有最终结论
		if thought.Action == "final_answer" {
			state.Set("react_final_answer", thought.ActionInput)
			state.Set("react_should_continue", false)
			return state, nil
		}

		// Step 2: Action
		if thought.Action != "" && thought.Action != "final_answer" {
			tool, exists := toolRegistry.Get(thought.Action)
			if !exists {
				state.Set("react_error", fmt.Sprintf("tool %s not found", thought.Action))
				continue
			}

			result, err := tool.Execute(ctx, thought.ActionInput)
			if err != nil {
				state.Set("react_observation", fmt.Sprintf("Tool execution error: %v", err))
			} else {
				state.Set("react_observation", result)
			}
		}

		// 更新对话历史
		history := state.GetString("react_messages")
		history += fmt.Sprintf("\nThought: %s\nObservation: %s",
			state.GetString("react_thought"),
			state.GetString("react_observation"),
		)
		state.Set("react_messages", history)
	}

	// 达到最大迭代次数
	state.Set("react_truncated", true)
	return state, nil
}

// ThoughtResult ReAct 思考结果
type ThoughtResult struct {
	Thought     string `json:"thought"`
	Action      string `json:"action"`       // 工具名或 "final_answer"
	ActionInput string `json:"action_input"` // 工具输入或最终答案
}

// buildThoughtPrompt 构建思考提示词
func buildThoughtPrompt(state graph.State) string {
	messages := state.GetString("messages_text")
	if messages == "" {
		messages = state.GetString("react_messages")
	}

	history := state.GetString("react_thought")
	if history != "" {
		history = "之前的思考：" + history + "\n"
	}

	observation := state.GetString("react_observation")
	if observation != "" {
		observation = "观察结果：" + observation + "\n"
	}

	return fmt.Sprintf(`你是一个心理健康分析专家。请分析以下对话记录，逐步思考并给出判断。

对话记录：
%s

%s%s请按以下 JSON 格式输出你的思考：

{
  "thought": "你的逐步思考过程（中文）",
  "action": "工具名称或 final_answer",
  "action_input": "如果使用工具，输入参数；如果 final_answer，填写最终结论"
}

可用工具：
- query_history: 查询用户历史对话（输入：时间范围，如 "7d"）
- query_knowledge: 查询心理学知识（输入：关键词）
- final_answer: 给出最终分析结论

规则：
1. 如果需要更多信息，选择合适的工具查询
2. 如果已经能得出结论，使用 final_answer
3. 思考要具体、专业，引用对话中的具体证据`, messages, history, observation)
}

// parseThought 解析 LLM 的思考结果
func parseThought(response string) *ThoughtResult {
	result := &ThoughtResult{
		Thought:     "无法解析思考结果",
		Action:      "final_answer",
		ActionInput: response,
	}

	// 尝试提取 JSON
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || end <= start {
		return result
	}

	jsonStr := response[start : end+1]
	if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
		// 解析失败，将完整响应当作最终答案
		result.ActionInput = response
	}

	return result
}
