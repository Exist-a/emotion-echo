// Package downstream — memory_grpc.go
//
// CL-08a · A5：BFF → ai-svc MemoryService 客户端（「我的记忆」自述入口 + 记忆图）。
//
// 为什么是独立窄接口（与 AgentProfileClient 同款理由）：
//   - 只有 gRPC，没有 HTTP 回退（ai-svc 的 HTTP 面没有这些端点）；
//   - conn 为 nil ⇒ 返回 nil，handler 侧显式 503（fail-closed），而不是静默走错路径。
package downstream

import (
	"context"

	emotionmemory "github.com/emotion-echo/shared/pkg/emotionmemory"

	"google.golang.org/grpc"
)

// MemoryNoteResult 自述条写入结果（BFF 直接回给前端）。
type MemoryNoteResult struct {
	NoteID  int64  `json:"noteId"`
	AckText string `json:"ackText"`
}

// MemoryGraphNode 记忆图节点。
type MemoryGraphNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Weight int32  `json:"weight"`
}

// MemoryGraphEdge 记忆图边（Label = predicate）。
type MemoryGraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// MemoryGraph 记忆图。
type MemoryGraph struct {
	Nodes []MemoryGraphNode `json:"nodes"`
	Edges []MemoryGraphEdge `json:"edges"`
}

// MemoryClient BFF → ai-svc 记忆客户端。
type MemoryClient interface {
	// CreateNote 写入一条自述并触发异步抽取；返回固定回执。
	CreateNote(ctx context.Context, userID int64, content string) (*MemoryNoteResult, error)
	// GetGraph 取用户记忆图（当前由 PG 事实渲染）。
	GetGraph(ctx context.Context, userID int64) (*MemoryGraph, error)
}

type memoryGRPCClient struct{ conn *grpc.ClientConn }

// NewMemoryClient conn 为 nil 时返回 nil（= 未装配，handler 侧 503）。
func NewMemoryClient(conn *grpc.ClientConn) MemoryClient {
	if conn == nil {
		return nil
	}
	return &memoryGRPCClient{conn: conn}
}

func (c *memoryGRPCClient) CreateNote(ctx context.Context, userID int64, content string) (*MemoryNoteResult, error) {
	cli := emotionmemory.NewMemoryServiceClient(c.conn)
	resp, err := cli.CreateNote(withUserID(ctx), &emotionmemory.CreateNoteRequest{
		UserId:  userID,
		Content: content,
	})
	if err != nil {
		return nil, wrapGRPCError(err, "create memory note")
	}
	return &MemoryNoteResult{NoteID: resp.GetNoteId(), AckText: resp.GetAckText()}, nil
}

func (c *memoryGRPCClient) GetGraph(ctx context.Context, userID int64) (*MemoryGraph, error) {
	cli := emotionmemory.NewMemoryServiceClient(c.conn)
	resp, err := cli.GetGraph(withUserID(ctx), &emotionmemory.GetGraphRequest{UserId: userID})
	if err != nil {
		return nil, wrapGRPCError(err, "get memory graph")
	}
	nodes := make([]MemoryGraphNode, 0, len(resp.GetNodes()))
	for _, n := range resp.GetNodes() {
		nodes = append(nodes, MemoryGraphNode{ID: n.GetId(), Label: n.GetLabel(), Kind: n.GetKind(), Weight: n.GetWeight()})
	}
	edges := make([]MemoryGraphEdge, 0, len(resp.GetEdges()))
	for _, e := range resp.GetEdges() {
		edges = append(edges, MemoryGraphEdge{Source: e.GetSource(), Target: e.GetTarget(), Label: e.GetLabel()})
	}
	return &MemoryGraph{Nodes: nodes, Edges: edges}, nil
}
