// internal/grpcserver/memory_graph_test.go
//
// CL-08a · A5：记忆图渲染（纯函数）。

package grpcserver

import (
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
)

func graphFact(subject, predicate, object string) model.MemoryFact {
	return model.MemoryFact{
		UserID:    1,
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		ValidFrom: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Status:    model.MemoryFactStatusActive,
	}
}

func TestBuildMemoryGraph_Empty(t *testing.T) {
	t.Parallel()
	g := buildMemoryGraph(1, nil)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("空事实应返回空图（前端显示空态），got %d nodes / %d edges", len(g.Nodes), len(g.Edges))
	}
}

func TestBuildMemoryGraph_SelfSubjectCentered(t *testing.T) {
	t.Parallel()
	g := buildMemoryGraph(1, []model.MemoryFact{graphFact("用户", "职业", "后端工程师")})

	if len(g.Nodes) != 2 {
		t.Fatalf("期望 2 个节点（我 + 实体），got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("期望 1 条边，got %d", len(g.Edges))
	}
	if g.Edges[0].Source != "user:1" {
		t.Errorf("subject=用户 ⇒ 边应从中心节点 user:1 出发，got %q", g.Edges[0].Source)
	}
	if g.Edges[0].Label != "职业" {
		t.Errorf("边标签应为 predicate，got %q", g.Edges[0].Label)
	}

	var self *struct{ label, kind string }
	for _, n := range g.Nodes {
		if n.Id == "user:1" {
			self = &struct{ label, kind string }{n.Label, n.Kind}
		}
	}
	if self == nil {
		t.Fatal("缺少中心节点 user:1")
	}
	if self.label != "我" || self.kind != "user" {
		t.Errorf("中心节点应为 我/user，got %q/%q", self.label, self.kind)
	}
}

func TestBuildMemoryGraph_NonSelfSubject(t *testing.T) {
	t.Parallel()
	g := buildMemoryGraph(1, []model.MemoryFact{graphFact("妈妈", "喜欢", "广场舞")})

	if len(g.Nodes) != 2 {
		t.Fatalf("非本人 subject 应产生 subject/object 两个实体节点，got %d", len(g.Nodes))
	}
	if g.Edges[0].Source != "entity:妈妈" || g.Edges[0].Target != "entity:广场舞" {
		t.Errorf("边应为 妈妈→广场舞，got %+v", g.Edges[0])
	}
}

func TestBuildMemoryGraph_DedupesEdges(t *testing.T) {
	t.Parallel()
	g := buildMemoryGraph(1, []model.MemoryFact{
		graphFact("用户", "职业", "后端工程师"),
		graphFact("用户", "职业", "后端工程师"),
	})
	if len(g.Edges) != 1 {
		t.Fatalf("重复事实不得产生重复边，got %d", len(g.Edges))
	}
}

func TestBuildMemoryGraph_WeightCountsFacts(t *testing.T) {
	t.Parallel()
	g := buildMemoryGraph(1, []model.MemoryFact{
		graphFact("用户", "职业", "后端工程师"),
		graphFact("用户", "爱好", "跑步"),
	})
	var selfWeight int32
	for _, n := range g.Nodes {
		if n.Id == "user:1" {
			selfWeight = n.Weight
		}
	}
	if selfWeight != 2 {
		t.Fatalf("中心节点权重应为关联事实数 2，got %d", selfWeight)
	}
}
