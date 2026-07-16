package graph

import (
	"context"
	"fmt"
	"sync"
)

// SequentialNode 顺序执行节点
type SequentialNode struct {
	id       string
	children []Node
}

// NewSequentialNode 创建顺序节点
func NewSequentialNode(id string, children ...Node) *SequentialNode {
	return &SequentialNode{
		id:       id,
		children: children,
	}
}

func (n *SequentialNode) GetID() string {
	return n.id
}

func (n *SequentialNode) Execute(ctx context.Context, state State) (State, error) {
	currentState := state
	for _, child := range n.children {
		newState, err := child.Execute(ctx, currentState)
		if err != nil {
			return currentState, err
		}
		currentState = newState
	}
	return currentState, nil
}

// ParallelNode 并行执行节点
type ParallelNode struct {
	id       string
	children []Node
}

// NewParallelNode 创建并行节点
func NewParallelNode(id string, children ...Node) *ParallelNode {
	return &ParallelNode{
		id:       id,
		children: children,
	}
}

func (n *ParallelNode) GetID() string {
	return n.id
}

func (n *ParallelNode) Execute(ctx context.Context, state State) (State, error) {
	type result struct {
		state State
		err   error
	}
	
	results := make(chan result, len(n.children))
	var wg sync.WaitGroup
	
	// 并行执行所有子节点
	for _, child := range n.children {
		wg.Add(1)
		go func(node Node) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					results <- result{state: state.Clone(), err: fmt.Errorf("node panic: %v", r)}
				}
			}()
			newState, err := node.Execute(ctx, state.Clone())
			results <- result{state: newState, err: err}
		}(child)
	}
	
	// 等待所有子节点完成
	wg.Wait()
	close(results)
	
	// 合并结果
	finalState := state
	for res := range results {
		if res.err != nil {
			return state, res.err
		}
		// 合并状态（后面的覆盖前面的）
		finalState = finalState.Merge(res.state)
	}
	
	return finalState, nil
}

// ConditionalNode 条件分支节点
type ConditionalNode struct {
	id       string
	branches []Branch
	defaultNode Node
}

// Branch 条件分支
type Branch struct {
	Condition EdgeCondition
	Node      Node
}

// NewConditionalNode 创建条件节点
func NewConditionalNode(id string, branches []Branch, defaultNode Node) *ConditionalNode {
	return &ConditionalNode{
		id:       id,
		branches: branches,
		defaultNode: defaultNode,
	}
}

func (n *ConditionalNode) GetID() string {
	return n.id
}

func (n *ConditionalNode) Execute(ctx context.Context, state State) (State, error) {
	// 依次检查条件
	for _, branch := range n.branches {
		if branch.Condition.Evaluate(state) {
			return branch.Node.Execute(ctx, state)
		}
	}
	
	// 执行默认分支
	if n.defaultNode != nil {
		return n.defaultNode.Execute(ctx, state)
	}
	
	return state, nil
}

// FunctionNode 函数节点（执行自定义函数）
type FunctionNode struct {
	id       string
	function func(ctx context.Context, state State) (State, error)
}

// NewFunctionNode 创建函数节点
func NewFunctionNode(id string, fn func(ctx context.Context, state State) (State, error)) *FunctionNode {
	return &FunctionNode{
		id:       id,
		function: fn,
	}
}

func (n *FunctionNode) GetID() string {
	return n.id
}

func (n *FunctionNode) Execute(ctx context.Context, state State) (State, error) {
	return n.function(ctx, state)
}

// LLMNode LLM调用节点
type LLMNode struct {
	id      string
	prompt  string
	callLLM func(ctx context.Context, prompt string) (string, error)
	parser  func(response string, state State) (State, error)
}

// NewLLMNode 创建LLM节点
func NewLLMNode(id string, prompt string, callLLM func(ctx context.Context, prompt string) (string, error), parser func(response string, state State) (State, error)) *LLMNode {
	return &LLMNode{
		id:      id,
		prompt:  prompt,
		callLLM: callLLM,
		parser:  parser,
	}
}

func (n *LLMNode) GetID() string {
	return n.id
}

func (n *LLMNode) Execute(ctx context.Context, state State) (State, error) {
	// 调用LLM
	response, err := n.callLLM(ctx, n.prompt)
	if err != nil {
		return state, err
	}
	
	// 解析响应
	if n.parser != nil {
		return n.parser(response, state)
	}
	
	return state, nil
}
