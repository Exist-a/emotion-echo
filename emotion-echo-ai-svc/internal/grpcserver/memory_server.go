// Package grpcserver — memory_server.go
//
// CL-08a · A5：MemoryService 的 ai-svc 实现。
//
//   - CreateNote：写入自述条 → **异步**抽取（不调大模型做回答）→ 立即返回固定回执。
//   - GetGraph：读 PG 事实渲染中心辐射图（D7）。
package grpcserver

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	emotionmemory "github.com/emotion-echo/shared/pkg/emotionmemory"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// MemoryNoteAckText 自述条固定回执（前端直接展示）。
	MemoryNoteAckText = "记忆已更新"
	// memoryNoteMaxRunes 单条自述长度上限（防止把整本书灌进抽取器）。
	memoryNoteMaxRunes = 4000
	// memoryGraphFactLimit 图渲染最多取多少条 active 事实。
	memoryGraphFactLimit = 500
)

// MemoryNoteExtractor 自述条抽取能力（窄函数类型，避免 grpcserver 直接依赖 logic 包）。
type MemoryNoteExtractor func(ctx context.Context, note *model.MemoryNote) error

type memoryServer struct {
	emotionmemory.UnimplementedMemoryServiceServer
	repo    repository.MemoryFactRepo
	extract MemoryNoteExtractor
	// graphDeps CL-08d：删除闭环的图侧依赖（nil = 图侧跳过，记 warn）
	graphDeps *memoryGraphDeps
}

func newMemoryServer(repo repository.MemoryFactRepo, extract MemoryNoteExtractor) *memoryServer {
	return &memoryServer{repo: repo, extract: extract}
}

// SetGraphDeps 注入删除闭环的图侧依赖（CL-08d；main.go 装配）。
func (s *memoryServer) SetGraphDeps(deps *memoryGraphDeps) { s.graphDeps = deps }

func (s *memoryServer) CreateNote(ctx context.Context, req *emotionmemory.CreateNoteRequest) (*emotionmemory.CreateNoteResponse, error) {
	uid, err := memoryCallerID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, status.Error(codes.InvalidArgument, "content required")
	}
	if utf8.RuneCountInString(content) > memoryNoteMaxRunes {
		return nil, status.Errorf(codes.InvalidArgument, "content too long (max %d runes)", memoryNoteMaxRunes)
	}

	note := &model.MemoryNote{
		UserID:        uid,
		Content:       content,
		ExtractStatus: model.MemoryExtractStatusPending,
	}
	if err := s.repo.CreateNote(ctx, note); err != nil {
		return nil, status.Errorf(codes.Internal, "create note: %v", err)
	}

	// 异步抽取：**请求先返回**（该页不需要大模型回答），抽取失败只体现为 note 状态。
	// context.WithoutCancel 保留 user id 等值但去掉请求取消/超时 —— 否则客户端断开即丢抽取。
	if s.extract != nil {
		n := *note
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("memory: async note extraction panic", "note_id", n.ID, "panic", r)
				}
			}()
			if err := s.extract(context.WithoutCancel(ctx), &n); err != nil {
				slog.Error("memory: async note extraction failed", "note_id", n.ID, "err", err)
			}
		}()
	}

	return &emotionmemory.CreateNoteResponse{NoteId: note.ID, AckText: MemoryNoteAckText}, nil
}

func (s *memoryServer) GetGraph(ctx context.Context, req *emotionmemory.GetGraphRequest) (*emotionmemory.GetGraphResponse, error) {
	uid, err := memoryCallerID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	facts, err := s.repo.ListActiveByUser(ctx, uid, memoryGraphFactLimit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list facts: %v", err)
	}
	return buildMemoryGraph(uid, facts), nil
}

// memoryCallerID 取**可信身份**并校验请求体。
//
// 安全边界（第二方核对抓出）：请求体里的 user_id 是客户端可传值，**不可信**；
// 权威身份来自 gRPC metadata 的 x-user-id（由 BFF/网关注入，拦截器已校验为正整数）。
// 因此：
//   - 没有 metadata ⇒ Unauthenticated（fail-closed，绝不回落到请求体）；
//   - 请求体给了不同的 user_id ⇒ PermissionDenied（越权尝试，显式拒绝而不是静默忽略）。
func memoryCallerID(ctx context.Context, bodyUserID int64) (int64, error) {
	uid, ok := grpcinterceptor.UserIDFromGRPCContext(ctx)
	if !ok || uid <= 0 {
		return 0, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}
	if bodyUserID != 0 && bodyUserID != uid {
		return 0, status.Error(codes.PermissionDenied, "user_id mismatch")
	}
	return uid, nil
}

// ---------------------------------------------------------------------------
// CL-08d：管理页列举 + 删除闭环编排。
// ---------------------------------------------------------------------------

// GraphDeleter 删除闭环的图侧能力（窄接口；生产 = internal/memory 的 gRPC writer）。
type GraphDeleter interface {
	DeleteUserData(ctx context.Context, userID int64) error
	RemoveEpisode(ctx context.Context, userID int64, episodeUUID string) error
}

// SummaryRefresher 摘要失效/刷新能力（删除后立即生效，注入才不会再带出）。
type SummaryRefresher func(ctx context.Context, userID int64) error

// RebuildFunc 全图重建（uuid 缺失的回落路径；cmd/memory-rebuild 共用编排）。
type RebuildFunc func(ctx context.Context, userID int64) (ok, failed int, err error)

// memoryGraphDeps 图侧依赖（main.go 注入；graph 为 nil ⇒ 图侧跳过并记 warn ——
// PG 仍是事实源，缺口由重建工具兜底）。
type memoryGraphDeps struct {
	graph   GraphDeleter
	refresh SummaryRefresher
	rebuild RebuildFunc
}

// ListMemory 管理页列举：active 事实 + 自述条（前端分节渲染）。
func (s *memoryServer) ListMemory(ctx context.Context, req *emotionmemory.ListMemoryRequest) (*emotionmemory.ListMemoryResponse, error) {
	uid, err := memoryCallerID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	resp := &emotionmemory.ListMemoryResponse{}
	facts, err := s.repo.ListActiveByUser(ctx, uid, memoryGraphFactLimit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list facts: %v", err)
	}
	for _, f := range facts {
		resp.Items = append(resp.Items, &emotionmemory.MemoryItem{
			Kind: "fact", Id: f.ID, Text: f.FactText,
			SourceKind: f.SourceKind, SourceId: f.SourceID,
			CreatedAt: f.ValidFrom.Format("2006-01-02"),
		})
	}
	notes, err := s.repo.ListNotes(ctx, uid, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list notes: %v", err)
	}
	for _, n := range notes {
		resp.Items = append(resp.Items, &emotionmemory.MemoryItem{
			Kind: "note", Id: n.ID, Text: n.Content,
			CreatedAt: n.CreatedAt.Format("2006-01-02"),
		})
	}
	return resp, nil
}

// DeleteMemory 删除闭环编排：PG 软删（事实源）→ 图随删 → 摘要刷新。
//
// 图侧失败**不让删除失败**（图是可重建索引；缺口由 cmd/memory-rebuild 兜底），
// 但会在 message 里如实标注（前端可提示"部分完成"）。
func (s *memoryServer) DeleteMemory(ctx context.Context, req *emotionmemory.DeleteMemoryRequest) (*emotionmemory.DeleteMemoryResponse, error) {
	uid, err := memoryCallerID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	scope := req.GetScope()
	switch scope {
	case "fact":
		return s.deleteByFact(ctx, uid, req.GetId())
	case "note":
		return s.deleteByNote(ctx, uid, req.GetId())
	case "all":
		return s.deleteAll(ctx, uid)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "scope must be fact|note|all, got %q", scope)
	}
}

// removeSourceFromGraphAsync 异步收敛图侧（CL-08d 运行时实测：图摘除/重建是
// LLM 级慢操作 —— 单条秒级、全图重建几十秒，同步做必撞 BFF 下游 5s 截止 ⇒
// 用户看到 502 但 PG 已删，且请求 ctx 取消会打断图操作与摘要刷新）。
//
// 语义与 forwardEpisodeToGraph 同范式：**PG 即刻生效（响应不等图）**，图侧
// best-effort 异步收敛；失败只记日志，缺口由 cmd/memory-rebuild 兜底。
// 摘要刷新同样用独立 ctx（旧实现挂在请求 ctx 上，取消后摘要不刷新 —— 实测抓到）。
func (s *memoryServer) removeSourceFromGraphAsync(uid int64, kind string, id int64) {
	if s.graphDeps == nil || s.graphDeps.graph == nil {
		slog.Warn("memory delete: graph writer not configured (PG deleted; graph left for rebuild)")
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("memory delete: graph op panicked (recovered)", "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		uuid, err := s.repo.GetEpisodeGraphUUID(ctx, kind, id)
		if err != nil {
			slog.Warn("memory delete: lookup graph uuid", "err", err)
		}
		if uuid != "" {
			if gerr := s.graphDeps.graph.RemoveEpisode(ctx, uid, uuid); gerr != nil {
				slog.Warn("memory delete: remove episode from graph", "uuid", uuid, "err", gerr)
			}
			return
		}
		// uuid 缺失（Phase 4 之前的旧行 / 上次回填失败）⇒ 整图重建兜底
		if s.graphDeps.rebuild == nil {
			slog.Warn("memory delete: graph uuid unknown and rebuild unavailable")
			return
		}
		okN, failedN, rerr := s.graphDeps.rebuild(ctx, uid)
		if rerr != nil {
			slog.Warn("memory delete: graph rebuild fallback failed", "err", rerr)
			return
		}
		slog.Info("memory delete: graph rebuilt as fallback", "user_id", uid, "ok", okN, "failed", failedN)
	}()
}

// refreshSummaryAsync 异步刷新摘要（独立 ctx；请求 ctx 已知会被 BFF 截止取消）。
func (s *memoryServer) refreshSummaryAsync(uid int64) {
	if s.graphDeps == nil || s.graphDeps.refresh == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.graphDeps.refresh(ctx, uid); err != nil {
			slog.Warn("memory delete: refresh summary", "user_id", uid, "err", err)
		}
	}()
}

func (s *memoryServer) deleteByFact(ctx context.Context, uid, factID int64) (*emotionmemory.DeleteMemoryResponse, error) {
	f, err := s.repo.GetFactByID(ctx, uid, factID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get fact: %v", err)
	}
	if f == nil {
		return &emotionmemory.DeleteMemoryResponse{Ok: true, Message: "already gone"}, nil
	}
	if _, err := s.repo.SoftDeleteFact(ctx, uid, factID); err != nil {
		return nil, status.Errorf(codes.Internal, "delete fact: %v", err)
	}
	// 删记忆 = 忘掉这件事 ⇒ 来源情节一并软删（重建不会复活；同源其他 fact 保留，
	// 其图上内容由 remove_episode 一并摘除 —— 已知边界，见 CL-08d 报告）
	if err := s.repo.SoftDeleteEpisode(ctx, f.SourceKind, f.SourceID); err != nil {
		return nil, status.Errorf(codes.Internal, "delete episode: %v", err)
	}
	// PG 即刻生效；图侧/摘要异步收敛（响应不等慢操作 —— 实测 5s 截止）
	s.removeSourceFromGraphAsync(uid, f.SourceKind, f.SourceID)
	s.refreshSummaryAsync(uid)
	return &emotionmemory.DeleteMemoryResponse{Ok: true, Message: "deleted"}, nil
}

func (s *memoryServer) deleteByNote(ctx context.Context, uid, noteID int64) (*emotionmemory.DeleteMemoryResponse, error) {
	n, err := s.repo.GetNoteByID(ctx, uid, noteID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get note: %v", err)
	}
	if n == nil || n.DeletedAt != nil {
		return &emotionmemory.DeleteMemoryResponse{Ok: true, Message: "already gone"}, nil
	}
	if _, err := s.repo.SoftDeleteNote(ctx, uid, noteID); err != nil {
		return nil, status.Errorf(codes.Internal, "delete note: %v", err)
	}
	if _, err := s.repo.SoftDeleteFactsBySource(ctx, uid, model.MemorySourceKindNote, noteID); err != nil {
		return nil, status.Errorf(codes.Internal, "delete note facts: %v", err)
	}
	if err := s.repo.SoftDeleteEpisode(ctx, model.MemorySourceKindNote, noteID); err != nil {
		return nil, status.Errorf(codes.Internal, "delete episode: %v", err)
	}
	s.removeSourceFromGraphAsync(uid, model.MemorySourceKindNote, noteID)
	s.refreshSummaryAsync(uid)
	return &emotionmemory.DeleteMemoryResponse{Ok: true, Message: "deleted"}, nil
}

func (s *memoryServer) deleteAll(ctx context.Context, uid int64) (*emotionmemory.DeleteMemoryResponse, error) {
	facts, notes, episodes, entities, err := s.repo.SoftDeleteAllForUser(ctx, uid)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete all: %v", err)
	}
	if s.graphDeps != nil && s.graphDeps.graph != nil {
		// 删全部 = drop 物理分图，O(1) 快路径；仍走异步（万一图库抖动也不拖响应）
		go func() {
			gctx, gcancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer gcancel()
			if gerr := s.graphDeps.graph.DeleteUserData(gctx, uid); gerr != nil {
				slog.Warn("memory delete-all: drop user graph", "user_id", uid, "err", gerr)
			}
		}()
	}
	s.refreshSummaryAsync(uid)
	slog.InfoContext(ctx, "memory delete-all done",
		"user_id", uid, "facts", facts, "notes", notes, "episodes", episodes, "entities", entities)
	return &emotionmemory.DeleteMemoryResponse{Ok: true, Message: "deleted"}, nil
}


