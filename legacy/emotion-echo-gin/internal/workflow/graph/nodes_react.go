package graph

import (
	"context"
	"fmt"
)

// ReActNode ReAct 循环节点
// 实现 Thought → Action → Observation 的循环模式
type ReActNode struct {
	id            string
	maxIterations int
	thoughtNode   Node
	actionNode    Node
	observationNode Node
}

// NewReActNode 创建 ReAct 节点
// maxIterations: 最大循环次数，防止死循环
// thoughtNode:   生成思考（LLM）
// actionNode:    执行工具
// observationNode: 整合观察结果
func NewReActNode(id string, maxIterations int, thoughtNode, actionNode, observationNode Node) *ReActNode {
	if maxIterations <= 0 {
		maxIterations = 5
	}
	return &ReActNode{
		id:              id,
		maxIterations:   maxIterations,
		thoughtNode:     thoughtNode,
		actionNode:      actionNode,
		observationNode: observationNode,
	}
}

func (n *ReActNode) GetID() string {
	return n.id
}

func (n *ReActNode) Execute(ctx context.Context, state State) (State, error) {
	currentState := state
	
	for i := 0; i < n.maxIterations; i++ {
		// Step 1: Thought（思考）
		thoughtState, err := n.thoughtNode.Execute(ctx, currentState)
		if err != nil {
			return currentState, fmt.Errorf("react thought failed (iteration %d): %w", i, err)
		}
		
		// 检查是否应该继续（thought 节点可以设置 should_continue 标志）
		shouldContinue := thoughtState.GetBool("react_should_continue")
		if !shouldContinue {
			// Thought 认为不需要继续，直接返回
			return thoughtState, nil
		}
		
		// Step 2: Action（执行工具）
		actionState, err := n.actionNode.Execute(ctx, thoughtState)
		if err != nil {
			return thoughtState, fmt.Errorf("react action failed (iteration %d): %w", i, err)
		}
		
		// Step 3: Observation（整合观察）
		observationState, err := n.observationNode.Execute(ctx, actionState)
		if err != nil {
			return actionState, fmt.Errorf("react observation failed (iteration %d): %w", i, err)
		}
		
		currentState = observationState
		currentState.Set("react_iteration", i+1)
	}
	
	// 达到最大迭代次数，标记为截断
	currentState.Set("react_truncated", true)
	return currentState, nil
}

// ToolNode 工具执行节点
type ToolNode struct {
	id     string
	tools  map[string]ToolExecutor
}

// ToolExecutor 工具执行函数
type ToolExecutor func(ctx context.Context, input string) (string, error)

// NewToolNode 创建工具节点
func NewToolNode(id string, tools map[string]ToolExecutor) *ToolNode {
	return &ToolNode{
		id:    id,
		tools: tools,
	}
}

func (n *ToolNode) GetID() string {
	return n.id
}

func (n *ToolNode) Execute(ctx context.Context, state State) (State, error) {
	toolName := state.GetString("react_tool_name")
	toolInput := state.GetString("react_tool_input")
	
	if toolName == "" {
		// 无工具调用，直接返回
		return state, nil
	}
	
	executor, exists := n.tools[toolName]
	if !exists {
		return state, fmt.Errorf("tool %s not found", toolName)
	}
	
	result, err := executor(ctx, toolInput)
	if err != nil {
		state.Set("react_tool_error", err.Error())
		return state, nil
	}
	
	state.Set("react_tool_result", result)
	state.Set("react_tool_name", "") // 清空，防止重复执行
	return state, nil
}
