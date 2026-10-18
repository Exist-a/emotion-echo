// Package downstream — agent_profile.go
//
// CL-06 · Phase 4（BFF 段）：BFF → user-svc **角色档案**契约（用户自定义 Agent·多角色）。
//
// 契约落点（源计划 §3.2）：角色档案归 **user-svc** 所有 ⇒ RPC 声明在 `proto/user.proto`
// （`ListAgentProfiles` / `UpsertAgentProfile` / `DeleteAgentProfile`）。
// ⚠️ **禁止**落 `proto/agent.proto` —— 那个文件名指的是 assessment-svc，不是用户角色。
//
// 为什么是**独立窄接口**而不是塞进 UserClient：
//   - `UserClient` 有 3 个测试替身（user_handler_test / auth_handler_test / avatar_handler_test）
//     与 1 个 HTTP 实现；把角色方法挂上去会强制它们实现与自身无关的 3 个方法；
//   - 角色客户端**只有 gRPC**：user-svc 当前**没有**角色相关 HTTP 端点
//     （只有 gRPC server 实现），故不提供 HTTP fallback —— 少了它会**显式报错**，
//     而不是静默走一条不存在的路径（fail-fast，参考 E2E-29 D-47 的信任链纪律）。
//
// 鉴权：metadata `x-user-id`（与 emotion_user / emotion_chat / emotion_query 一致）；
// user-svc 侧按该 id 做**归属过滤**（仓储层每个方法都带 user_id ⇒ 跨用户 IDOR 在读侧即被挡）。
package downstream

import (
	"context"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"

	"google.golang.org/grpc"
)

// AgentProfile 角色档案（JSON tag 供 BFF 直接回给前端，camelCase 与本仓前端契约一致）
type AgentProfile struct {
	ID         int64    `json:"id"`
	UserID     int64    `json:"userId"`
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

// AgentProfileUpsertReq 新建 / 更新角色请求。
// **id = 0 ⇒ 新建**；id > 0 ⇒ 更新（必须属于当前用户，越权由 user-svc 仓储层拒绝）。
type AgentProfileUpsertReq struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Gender     string   `json:"gender"`
	Persona    string   `json:"persona"`
	Tone       string   `json:"tone"`
	Boundaries []string `json:"boundaries"`
	Voice      string   `json:"voice"`
	IsDefault  bool     `json:"isDefault"`
	TemplateID string   `json:"templateId,omitempty"`
}

// AgentProfileClient BFF → user-svc 角色档案客户端
type AgentProfileClient interface {
	// ListAgentProfiles 列出当前用户的角色档案（含 get-or-create：首次读会物化默认「小E」）
	ListAgentProfiles(ctx context.Context) ([]AgentProfile, error)
	// UpsertAgentProfile 新建（req.ID=0）或更新（req.ID>0）
	UpsertAgentProfile(ctx context.Context, req AgentProfileUpsertReq) (*AgentProfile, error)
	// DeleteAgentProfile 软删除（幂等）
	DeleteAgentProfile(ctx context.Context, agentID int64) error
}

// agentProfileGRPCClient 是 AgentProfileClient 的 gRPC 实现
type agentProfileGRPCClient struct {
	conn *grpc.ClientConn
}

// NewAgentProfileGRPCClient 构造（需已建立的 user-svc gRPC 连接；conn=nil → 返 nil）
func NewAgentProfileGRPCClient(conn *grpc.ClientConn) AgentProfileClient {
	if conn == nil {
		return nil
	}
	return &agentProfileGRPCClient{conn: conn}
}

// ListAgentProfiles gRPC RPC
func (c *agentProfileGRPCClient) ListAgentProfiles(ctx context.Context) ([]AgentProfile, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.ListAgentProfiles(withUserID(ctx), &emotionuser.ListAgentProfilesRequest{})
	if err != nil {
		return nil, wrapGRPCError(err, "list agent profiles")
	}
	items := resp.GetItems()
	out := make([]AgentProfile, 0, len(items))
	for _, p := range items {
		out = append(out, *fromProtoAgentProfile(p))
	}
	return out, nil
}

// UpsertAgentProfile gRPC RPC
func (c *agentProfileGRPCClient) UpsertAgentProfile(ctx context.Context, req AgentProfileUpsertReq) (*AgentProfile, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.UpsertAgentProfile(withUserID(ctx), &emotionuser.UpsertAgentProfileRequest{
		Id:         req.ID,
		Name:       req.Name,
		Gender:     req.Gender,
		Persona:    req.Persona,
		Tone:       req.Tone,
		Boundaries: req.Boundaries,
		Voice:      req.Voice,
		IsDefault:  req.IsDefault,
		TemplateId: req.TemplateID,
	})
	if err != nil {
		return nil, wrapGRPCError(err, "upsert agent profile")
	}
	return fromProtoAgentProfile(resp), nil
}

// DeleteAgentProfile gRPC RPC
func (c *agentProfileGRPCClient) DeleteAgentProfile(ctx context.Context, agentID int64) error {
	cli := emotionuser.NewUserServiceClient(c.conn)
	_, err := cli.DeleteAgentProfile(withUserID(ctx), &emotionuser.DeleteAgentProfileRequest{Id: agentID})
	if err != nil {
		return wrapGRPCError(err, "delete agent profile")
	}
	return nil
}

// ============ proto → types 转换 ============

func fromProtoAgentProfile(p *emotionuser.AgentProfile) *AgentProfile {
	if p == nil {
		return nil
	}
	return &AgentProfile{
		ID:         p.GetId(),
		UserID:     p.GetUserId(),
		Name:       p.GetName(),
		Gender:     p.GetGender(),
		Persona:    p.GetPersona(),
		Tone:       p.GetTone(),
		Boundaries: p.GetBoundaries(),
		Voice:      p.GetVoice(),
		IsDefault:  p.GetIsDefault(),
		TemplateID: p.GetTemplateId(),
		CreatedAt:  p.GetCreatedAt(),
		UpdatedAt:  p.GetUpdatedAt(),
	}
}
