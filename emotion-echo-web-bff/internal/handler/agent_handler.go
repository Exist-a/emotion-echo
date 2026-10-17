// Package handler — agent_handler.go
//
// CL-06 · Phase 4：BFF 角色档案端点（用户自定义 Agent·多角色）。
//
// 端点（网关侧由既有 catch-all `/api/v1/*` 覆盖，无需新增 APISIX 路由）：
//
//	GET    /api/v1/agents        → {agents: [...], total: n}
//	POST   /api/v1/agents        → 档案本体（id=0 新建 / >0 更新）
//	DELETE /api/v1/agents/:id    → {success, id}
//
// 本层职责只有两件：**鉴权透传**（x-user-id → 下游）与**形状转换**。
// 字段上限、每户 ≤5、软删除、默认「小E」物化全在 user-svc（`agentlogic.go`）——
// 单一事实源，避免 BFF 与 svc 两套校验漂移（本仓 A1/A4 类"修错地方"的教训）。
//
// 未装配客户端 ⇒ 503（fail-closed）。角色服务不可用时应"明确不可用"，
// 而不是返回空列表 —— 后者会被前端读成"你还没有任何角色"，诱导用户重复创建。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// AgentHandler 处理 /api/v1/agents/* 端点
type AgentHandler struct {
	agents downstream.AgentProfileClient
}

// NewAgentHandler 构造（agents 为 nil ⇒ 所有端点 503）
func NewAgentHandler(agents downstream.AgentProfileClient) *AgentHandler {
	return &AgentHandler{agents: agents}
}

// Register 注册路由
func (h *AgentHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/agents", h.list)
	r.POST("/api/v1/agents", h.upsert)
	r.DELETE("/api/v1/agents/:id", h.remove)
}

// list GET /api/v1/agents
//
// 含 get-or-create：首次读会物化默认「小E」（user-svc 侧行为，见 agentlogic.go）。
func (h *AgentHandler) list(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	profiles, err := h.agents.ListAgentProfiles(session.WithRequestAuth(c))
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"agents": toAgentVMs(profiles), "total": len(profiles)})
}

// upsert POST /api/v1/agents
//
// 请求体即 `downstream.AgentProfileUpsertReq`（id=0 新建）。字段级校验在 user-svc，
// 这里的 400 只覆盖"体不是合法 JSON"这一种情况。
func (h *AgentHandler) upsert(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var req downstream.AgentProfileUpsertReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	saved, err := h.agents.UpsertAgentProfile(session.WithRequestAuth(c), req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	if saved == nil {
		Fail(c, http.StatusBadGateway, 1, "downstream: upsert agent profile returned empty")
		return
	}
	OK(c, toAgentVM(saved))
}

// remove DELETE /api/v1/agents/:id（软删除，幂等）
func (h *AgentHandler) remove(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid agent id")
		return
	}
	if err := h.agents.DeleteAgentProfile(session.WithRequestAuth(c), id); err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"success": true, "id": id})
}

// ready 未装配客户端时回 503 并返回 false（三个端点共用）。
func (h *AgentHandler) ready(c *gin.Context) bool {
	if h.agents == nil {
		Fail(c, http.StatusServiceUnavailable, 1, "agent profile service unavailable")
		return false
	}
	return true
}

// AgentVM 前端角色档案形状。
//
// 与 downstream.AgentProfile 的差别只在**不给前端 userId**（该值是服务端归属用的，
// 回给前端没有用途，还多一处"前端可以篡改 userId"的误读空间）。
type AgentVM struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Gender     string   `json:"gender"`
	Persona    string   `json:"persona"`
	Tone       string   `json:"tone"`
	Boundaries []string `json:"boundaries"`
	Voice      string   `json:"voice"`
	IsDefault  bool     `json:"isDefault"`
	TemplateID string   `json:"templateId,omitempty"`
	CreatedAt  int64    `json:"createdAt,omitempty"`
	UpdatedAt  int64    `json:"updatedAt,omitempty"`
}

func toAgentVM(p *downstream.AgentProfile) AgentVM {
	if p == nil {
		return AgentVM{}
	}
	return AgentVM{
		ID:         p.ID,
		Name:       p.Name,
		Gender:     p.Gender,
		Persona:    p.Persona,
		Tone:       p.Tone,
		Boundaries: p.Boundaries,
		Voice:      p.Voice,
		IsDefault:  p.IsDefault,
		TemplateID: p.TemplateID,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}
}

func toAgentVMs(ps []downstream.AgentProfile) []AgentVM {
	out := make([]AgentVM, 0, len(ps))
	for i := range ps {
		out = append(out, toAgentVM(&ps[i]))
	}
	return out
}
