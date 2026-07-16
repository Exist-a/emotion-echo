// Package graph 提供 DAG 工作流执行引擎
//
// 支持节点类型：
//   - SequentialNode: 顺序执行
//   - ParallelNode:   并行执行（goroutine）
//   - ConditionalNode: 条件分支
//   - LoopNode:       ReAct 循环
//
// 特性：
//   - 迭代执行（避免递归栈溢出）
//   - 检查点持久化（支持断点续跑和审计回溯）
package graph

import (
	"context"
	"fmt"
	"time"
)

// Graph 有向无环图执行引擎（迭代实现）
type Graph struct {
	ID          string
	Nodes       map[string]Node
	Edges       map[string][]Edge
	Checkpointer Checkpointer
}

// NewGraph 创建新的图
func NewGraph(id string, checkpointer Checkpointer) *Graph {
	return &Graph{
		ID:           id,
		Nodes:        make(map[string]Node),
		Edges:        make(map[string][]Edge),
		Checkpointer: checkpointer,
	}
}

// AddNode 添加节点
func (g *Graph) AddNode(node Node) {
	g.Nodes[node.GetID()] = node
}

// AddEdge 添加边
func (g *Graph) AddEdge(from string, to string, condition EdgeCondition) {
	g.Edges[from] = append(g.Edges[from], Edge{To: to, Condition: condition})
}

// GetNodes 获取所有节点
func (g *Graph) GetNodes() []Node {
	nodes := make([]Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// Execute 执行图（迭代实现，避免递归栈溢出）
func (g *Graph) Execute(ctx context.Context, runID string, initialState State) (State, error) {
	// 环检测
	if g.hasCycle() {
		return initialState, fmt.Errorf("graph contains cycle")
	}

	state := initialState
	step := 0
	maxSteps := len(g.Nodes) * 10 // 最大步数限制，防止无限循环

	// 尝试恢复检查点
	if g.Checkpointer != nil {
		savedStep, savedState, err := g.Checkpointer.LoadLatest(ctx, runID)
		if err == nil && savedState != nil {
			step = savedStep
			state = savedState
		}
	}

	// 找到入口节点
	entryNodes := g.findEntryNodes()
	if len(entryNodes) == 0 {
		return state, fmt.Errorf("no entry nodes found")
	}

	// 使用栈模拟递归（DFS）
	type stackItem struct {
		nodeID string
		state  State
	}

	stack := make([]stackItem, 0, len(g.Nodes))
	
	// 将所有入口节点入栈
	for _, entryID := range entryNodes {
		stack = append(stack, stackItem{nodeID: entryID, state: state})
	}

	for len(stack) > 0 {
		step++
		if step > maxSteps {
			return state, fmt.Errorf("max steps exceeded (%d), possible infinite loop", maxSteps)
		}

		// 弹出栈顶
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		node, exists := g.Nodes[item.nodeID]
		if !exists {
			return item.state, fmt.Errorf("node %s not found", item.nodeID)
		}

		// 执行节点
		newState, err := node.Execute(ctx, item.state)
		if err != nil {
			return newState, fmt.Errorf("node %s execution failed: %w", item.nodeID, err)
		}

		// 保存检查点
		if g.Checkpointer != nil {
			if err := g.Checkpointer.Save(ctx, runID, step, newState); err != nil {
				// 检查点保存失败不中断执行，仅记录日志
				// TODO: 接入日志框架
				_ = err
			}
		}

		// 获取出边并按优先级入栈（后入栈的先执行）
		edges, hasEdges := g.Edges[item.nodeID]
		if !hasEdges || len(edges) == 0 {
			continue
		}

		// 反向遍历，保证正序执行
		for i := len(edges) - 1; i >= 0; i-- {
			edge := edges[i]
			if edge.Condition != nil && !edge.Condition.Evaluate(newState) {
				continue
			}
			stack = append(stack, stackItem{nodeID: edge.To, state: newState})
		}
	}

	return state, nil
}

// ExecuteWithTimeout 带超时的执行
func (g *Graph) ExecuteWithTimeout(ctx context.Context, runID string, initialState State, timeout time.Duration) (State, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return g.Execute(ctx, runID, initialState)
}

// findEntryNodes 找到入口节点（没有入边的节点）
func (g *Graph) findEntryNodes() []string {
	hasIncoming := make(map[string]bool)
	for _, edges := range g.Edges {
		for _, edge := range edges {
			hasIncoming[edge.To] = true
		}
	}

	var entryNodes []string
	for id := range g.Nodes {
		if !hasIncoming[id] {
			entryNodes = append(entryNodes, id)
		}
	}

	return entryNodes
}

// hasCycle 使用 DFS 检测图中是否存在环
// 返回 true 表示存在环
func (g *Graph) hasCycle() bool {
	// 0 = 未访问(白色), 1 = 在递归栈中(灰色), 2 = 已处理完(黑色)
	color := make(map[string]int, len(g.Nodes))

	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		color[nodeID] = 1 // 标记为灰色（在递归栈中）

		for _, edge := range g.Edges[nodeID] {
			toID := edge.To
			if color[toID] == 1 {
				// 遇到灰色节点，说明存在环
				return true
			}
			if color[toID] == 0 {
				// 白色节点，继续 DFS
				if dfs(toID) {
					return true
				}
			}
			// 黑色节点，已处理完，无需处理
		}

		color[nodeID] = 2 // 标记为黑色（已处理完）
		return false
	}

	// 对所有节点进行 DFS（处理不连通的情况）
	for nodeID := range g.Nodes {
		if color[nodeID] == 0 {
			if dfs(nodeID) {
				return true
			}
		}
	}

	return false
}

// Node 节点接口
type Node interface {
	GetID() string
	Execute(ctx context.Context, state State) (State, error)
}

// Edge 边
type Edge struct {
	To        string
	Condition EdgeCondition
}

// EdgeCondition 边条件接口
type EdgeCondition interface {
	Evaluate(state State) bool
}
