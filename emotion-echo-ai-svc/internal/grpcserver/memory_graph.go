// Package grpcserver — memory_graph.go
//
// CL-08a · A5：把 PG 事实渲染成前端可画的中心辐射图（纯函数）。
//
// 形状（用户裁定：中心辐射图）：
//
//	用户本人（"我"）在中心；每条事实是一条 主体 --(predicate)--> 客体 的边；
//	subject 通常是"用户"，此时直接指向中心节点；其余实体各自成节点。
//
// 当前数据源 = PG 事实（用户裁定 D7）；CL-08c 图后端上线后**只换数据源**。
package grpcserver

import (
	"fmt"

	"emotion-echo-ai-svc/internal/model"

	emotionmemory "github.com/emotion-echo/shared/pkg/emotionmemory"
)

// memoryUserNodeID 用户本人节点的稳定 id（与实体节点区分）。
func memoryUserNodeID(userID int64) string { return fmt.Sprintf("user:%d", userID) }

// memoryEntityNodeID 实体节点的稳定 id。
func memoryEntityNodeID(label string) string { return "entity:" + label }

// isSelfSubject 判断 subject 是否指用户本人。
func isSelfSubject(subject string) bool {
	switch subject {
	case "用户", "我", "本人", "自己":
		return true
	}
	return false
}

// buildMemoryGraph 把 active 事实渲染成图。
//
// 空输入 ⇒ 空图（前端显示"还没有记忆"空态，而不是一个孤立的中心点）。
func buildMemoryGraph(userID int64, facts []model.MemoryFact) *emotionmemory.GetGraphResponse {
	resp := &emotionmemory.GetGraphResponse{}
	if len(facts) == 0 {
		return resp
	}

	selfID := memoryUserNodeID(userID)
	nodeIndex := map[string]*emotionmemory.MemoryNode{}
	addNode := func(id, label, kind string) *emotionmemory.MemoryNode {
		if n, ok := nodeIndex[id]; ok {
			return n
		}
		n := &emotionmemory.MemoryNode{Id: id, Label: label, Kind: kind}
		nodeIndex[id] = n
		resp.Nodes = append(resp.Nodes, n)
		return n
	}
	edgeSeen := map[string]struct{}{}

	for _, f := range facts {
		subjectID := selfID
		if !isSelfSubject(f.Subject) {
			subjectID = memoryEntityNodeID(f.Subject)
			addNode(subjectID, f.Subject, "subject")
		} else {
			addNode(selfID, "我", "user")
		}
		objectID := memoryEntityNodeID(f.Object)
		addNode(objectID, f.Object, "object")

		key := subjectID + "\x00" + objectID + "\x00" + f.Predicate
		if _, dup := edgeSeen[key]; dup {
			continue
		}
		edgeSeen[key] = struct{}{}
		resp.Edges = append(resp.Edges, &emotionmemory.MemoryEdge{
			Source: subjectID,
			Target: objectID,
			Label:  f.Predicate,
		})
		if n, ok := nodeIndex[subjectID]; ok {
			n.Weight++
		}
		if n, ok := nodeIndex[objectID]; ok {
			n.Weight++
		}
	}
	return resp
}
