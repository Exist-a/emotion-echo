// Package logic — agentlogic.go
//
// CL-06 Phase 2b（user-svc 服务层）：用户自定义角色（Agent）的业务规则。
//
// **服务端强制**（客户端绕不过；HTTP 与 gRPC 共用本层）：
//   - 每户活跃角色 ≤ `MaxAgentProfilesPerUser`（默认「小E」计入）；
//   - 字段上限按**字符**计（中文按字，不按字节）；
//   - `voice` 白名单 = 8 个 CosyVoice2 预置音色（空值回落默认）；
//   - 更新 / 删除前必须按 (属主, id) 校验 ⇒ 防越权（IDOR）；
//   - 默认「小E」**只在"该用户从未有过任何角色"时**物化（含软删除计数判定）——
//     否则用户把角色删光后再读列表会被重新物化 ⇒ 删除形同虚设（该点由
//     `TestListAgentProfiles_AfterDeletingDefault_DoesNotReseed` 钉死）。
package logic

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"emotion-echo-user-svc/internal/middleware"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"
)

const (
	// MaxAgentProfilesPerUser 每户活跃角色上限（含默认「小E」）
	MaxAgentProfilesPerUser = 5
	// AgentNameMaxRunes / AgentPersonaMaxRunes / AgentToneMaxRunes 字段上限（按字符）
	AgentNameMaxRunes    = 20
	AgentPersonaMaxRunes = 500
	AgentToneMaxRunes    = 100
	// AgentBoundariesMax / AgentBoundaryMaxRunes 负向约束条数与单条上限
	AgentBoundariesMax    = 5
	AgentBoundaryMaxRunes = 50

	// DefaultAgentVoice 默认音色（与本仓 BFF 的全局默认一致：anna 女声）
	DefaultAgentVoice = "FunAudioLLM/CosyVoice2-0.5B:anna"

	// DefaultAgentName / DefaultAgentPersona / DefaultAgentTone —— 默认「小E」的初始值。
	//
	// ⚠️ **文案待用户认可**（CL-06 计划 §8 N2：属用户可感内容，执行侧不自拟）——
	// 此处是"可用的最小实现"，用户过目后可直接改这几个常量（改它们不影响任何逻辑）。
	DefaultAgentName    = "小E"
	DefaultAgentPersona = "温柔、愿意倾听的陪伴者。回应简短自然，表达理解、不评判、不说教。"
	DefaultAgentTone    = "温柔、口语化"
)

// AllowedAgentVoiceList 8 个 CosyVoice2 预置音色（男声 4 / 女声 4）。
// 顺序即前端下拉的展示顺序。
var AllowedAgentVoiceList = []string{
	"FunAudioLLM/CosyVoice2-0.5B:anna",
	"FunAudioLLM/CosyVoice2-0.5B:bella",
	"FunAudioLLM/CosyVoice2-0.5B:claire",
	"FunAudioLLM/CosyVoice2-0.5B:diana",
	"FunAudioLLM/CosyVoice2-0.5B:alex",
	"FunAudioLLM/CosyVoice2-0.5B:benjamin",
	"FunAudioLLM/CosyVoice2-0.5B:charles",
	"FunAudioLLM/CosyVoice2-0.5B:david",
}

var allowedAgentVoices = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllowedAgentVoiceList))
	for _, v := range AllowedAgentVoiceList {
		m[v] = struct{}{}
	}
	return m
}()

// 业务 sentinel errors（grpcserver 侧映射为 gRPC code；HTTP 侧按 statusFor 映射）
var (
	// ErrUnauthorized 上下文中没有用户身份
	ErrUnauthorized = errors.New("unauthorized: missing user id in context")
	// ErrAgentNotFound 角色不存在 / 不属于当前用户 / 已被删除
	ErrAgentNotFound = errors.New("agent profile not found")
	// ErrAgentLimitReached 已达每户上限
	ErrAgentLimitReached = errors.New("agent profile limit reached")
)

// currentUserID 从 ctx 取当前用户（与 GetMeLogic 同款约定：middleware 注入）。
func currentUserID(ctx context.Context) (int64, error) {
	uid, ok := ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid == 0 {
		return 0, ErrUnauthorized
	}
	return uid, nil
}

// validateAgentProfile 校验并**规范化**（trim / 丢空项 / 音色回落）。
//
// 越界一律返回 ErrValidation（**拒绝保存**，不做静默截断 —— 静默截断会让用户
// 以为写进去了）。
func validateAgentProfile(req *types.UpsertAgentProfileReq) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Gender = strings.TrimSpace(req.Gender)
	req.Persona = strings.TrimSpace(req.Persona)
	req.Tone = strings.TrimSpace(req.Tone)
	req.TemplateID = strings.TrimSpace(req.TemplateID)

	if req.Name == "" || utf8.RuneCountInString(req.Name) > AgentNameMaxRunes {
		return ErrValidation
	}
	if utf8.RuneCountInString(req.Persona) > AgentPersonaMaxRunes {
		return ErrValidation
	}
	if utf8.RuneCountInString(req.Tone) > AgentToneMaxRunes {
		return ErrValidation
	}

	cleaned := make([]string, 0, len(req.Boundaries))
	for _, b := range req.Boundaries {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if utf8.RuneCountInString(b) > AgentBoundaryMaxRunes {
			return ErrValidation
		}
		cleaned = append(cleaned, b)
	}
	if len(cleaned) > AgentBoundariesMax {
		return ErrValidation
	}
	req.Boundaries = cleaned

	if req.Voice == "" {
		req.Voice = DefaultAgentVoice
	}
	if _, ok := allowedAgentVoices[req.Voice]; !ok {
		return ErrValidation
	}
	return nil
}

func toAgentProfileView(p *model.UserAgentProfile) types.AgentProfileView {
	if p == nil {
		return types.AgentProfileView{}
	}
	return types.AgentProfileView{
		ID:         p.ID,
		UserID:     p.UserID,
		Name:       p.Name,
		Gender:     p.Gender,
		Persona:    p.Persona,
		Tone:       p.Tone,
		Boundaries: p.Boundaries,
		Voice:      p.Voice,
		IsDefault:  p.IsDefault,
		TemplateID: p.TemplateID,
		CreatedAt:  p.CreatedAt.Unix(),
		UpdatedAt:  p.UpdatedAt.Unix(),
	}
}

// =====================================================
// ListAgentProfiles
// =====================================================

type ListAgentProfilesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAgentProfilesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAgentProfilesLogic {
	return &ListAgentProfilesLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListAgentProfiles 列出当前用户的角色；**从未建过任何角色**时先物化默认「小E」。
func (l *ListAgentProfilesLogic) ListAgentProfiles(_ *types.ListAgentProfilesReq) (*types.ListAgentProfilesResp, error) {
	uid, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.AgentRepo == nil {
		return nil, errors.New("agent repository not initialized")
	}

	// get-or-create 的判定口径：**含软删除**的总数为 0 才算"从未有过"。
	// （用户主动删光 ⇒ CountAll > 0 ⇒ 不再物化，尊重用户的删除。）
	all, err := l.svcCtx.AgentRepo.CountAll(l.ctx, uid)
	if err != nil {
		return nil, err
	}
	if all == 0 {
		if _, err := l.svcCtx.AgentRepo.Create(l.ctx, &model.UserAgentProfile{
			UserID:     uid,
			Name:       DefaultAgentName,
			Gender:     "女",
			Persona:    DefaultAgentPersona,
			Tone:       DefaultAgentTone,
			Voice:      DefaultAgentVoice,
			IsDefault:  true,
			Boundaries: []string{},
		}); err != nil {
			return nil, err
		}
	}

	items, err := l.svcCtx.AgentRepo.ListByUserID(l.ctx, uid)
	if err != nil {
		return nil, err
	}
	views := make([]types.AgentProfileView, 0, len(items))
	for _, it := range items {
		views = append(views, toAgentProfileView(it))
	}
	return &types.ListAgentProfilesResp{Items: views, Total: len(views)}, nil
}

// =====================================================
// UpsertAgentProfile
// =====================================================

type UpsertAgentProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpsertAgentProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpsertAgentProfileLogic {
	return &UpsertAgentProfileLogic{ctx: ctx, svcCtx: svcCtx}
}

// UpsertAgentProfile id=0 新建；id>0 更新（必须属于当前用户）。
func (l *UpsertAgentProfileLogic) UpsertAgentProfile(req *types.UpsertAgentProfileReq) (*types.AgentProfileView, error) {
	if req == nil {
		return nil, ErrValidation
	}
	uid, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.AgentRepo == nil {
		return nil, errors.New("agent repository not initialized")
	}
	if err := validateAgentProfile(req); err != nil {
		return nil, err
	}

	if req.ID == 0 {
		active, err := l.svcCtx.AgentRepo.CountActive(l.ctx, uid)
		if err != nil {
			return nil, err
		}
		if active >= MaxAgentProfilesPerUser {
			return nil, ErrAgentLimitReached
		}
		created, err := l.svcCtx.AgentRepo.Create(l.ctx, &model.UserAgentProfile{
			UserID:     uid,
			Name:       req.Name,
			Gender:     req.Gender,
			Persona:    req.Persona,
			Tone:       req.Tone,
			Boundaries: req.Boundaries,
			Voice:      req.Voice,
			IsDefault:  req.IsDefault,
			TemplateID: req.TemplateID,
		})
		if err != nil {
			return nil, err
		}
		view := toAgentProfileView(created)
		return &view, nil
	}

	// 更新：先按 (属主, id) 取 ⇒ 不存在/他人/已删 一律"找不到"
	existing, err := l.svcCtx.AgentRepo.GetByID(l.ctx, uid, req.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrAgentNotFound
	}
	existing.Name = req.Name
	existing.Gender = req.Gender
	existing.Persona = req.Persona
	existing.Tone = req.Tone
	existing.Boundaries = req.Boundaries
	existing.Voice = req.Voice
	existing.IsDefault = req.IsDefault
	existing.TemplateID = req.TemplateID
	if err := l.svcCtx.AgentRepo.Update(l.ctx, existing); err != nil {
		return nil, err
	}
	view := toAgentProfileView(existing)
	return &view, nil
}

// =====================================================
// DeleteAgentProfile
// =====================================================

type DeleteAgentProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteAgentProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteAgentProfileLogic {
	return &DeleteAgentProfileLogic{ctx: ctx, svcCtx: svcCtx}
}

// DeleteAgentProfile 软删除（幂等语义：已删 / 不存在 对外统一按"找不到"）。
func (l *DeleteAgentProfileLogic) DeleteAgentProfile(req *types.DeleteAgentProfileReq) (*types.DeleteAgentProfileResp, error) {
	if req == nil || req.ID <= 0 {
		return nil, ErrValidation
	}
	uid, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.AgentRepo == nil {
		return nil, errors.New("agent repository not initialized")
	}

	existing, err := l.svcCtx.AgentRepo.GetByID(l.ctx, uid, req.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrAgentNotFound
	}
	if err := l.svcCtx.AgentRepo.SoftDelete(l.ctx, uid, req.ID); err != nil {
		return nil, err
	}
	return &types.DeleteAgentProfileResp{Deleted: true}, nil
}

// 保证 repository 包被引用（ErrValidation 定义在 authlogic.go，此处只做编译期提示）
var _ = repository.ErrNotFound
