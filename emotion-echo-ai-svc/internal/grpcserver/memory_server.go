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
	"unicode/utf8"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	emotionmemory "github.com/emotion-echo/shared/pkg/emotionmemory"
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
}

func newMemoryServer(repo repository.MemoryFactRepo, extract MemoryNoteExtractor) *memoryServer {
	return &memoryServer{repo: repo, extract: extract}
}

func (s *memoryServer) CreateNote(ctx context.Context, req *emotionmemory.CreateNoteRequest) (*emotionmemory.CreateNoteResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id required")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, status.Error(codes.InvalidArgument, "content required")
	}
	if utf8.RuneCountInString(content) > memoryNoteMaxRunes {
		return nil, status.Errorf(codes.InvalidArgument, "content too long (max %d runes)", memoryNoteMaxRunes)
	}

	note := &model.MemoryNote{
		UserID:        req.GetUserId(),
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
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id required")
	}
	facts, err := s.repo.ListActiveByUser(ctx, req.GetUserId(), memoryGraphFactLimit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list facts: %v", err)
	}
	return buildMemoryGraph(req.GetUserId(), facts), nil
}
