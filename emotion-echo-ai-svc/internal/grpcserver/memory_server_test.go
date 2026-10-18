// internal/grpcserver/memory_server_test.go
//
// CL-08a · A5：MemoryService 行为契约（假仓储 + 假抽取器，不连 DB/LLM）。

package grpcserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	emotionmemory "github.com/emotion-echo/shared/pkg/emotionmemory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMemoryServer_CreateNote_Success(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	called := make(chan int64, 1)
	srv := newMemoryServer(repo, func(ctx context.Context, note *model.MemoryNote) error {
		called <- note.ID
		return nil
	})

	resp, err := srv.CreateNote(context.Background(), &emotionmemory.CreateNoteRequest{
		UserId: 3, Content: "  前年我一个人去了西藏  ",
	})
	if err != nil {
		t.Fatalf("CreateNote error: %v", err)
	}
	if resp.NoteId == 0 {
		t.Fatal("必须返回 note_id")
	}
	if resp.AckText != MemoryNoteAckText {
		t.Errorf("ack = %q, want %q", resp.AckText, MemoryNoteAckText)
	}

	stored, err := repo.GetNote(context.Background(), resp.NoteId)
	if err != nil || stored == nil {
		t.Fatalf("note 未落库: %v", err)
	}
	if stored.Content != "前年我一个人去了西藏" {
		t.Errorf("content 应 trim 后落库，got %q", stored.Content)
	}

	select {
	case id := <-called:
		if id != resp.NoteId {
			t.Errorf("异步抽取的 note id = %d, want %d", id, resp.NoteId)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CreateNote 后必须异步触发抽取")
	}
}

func TestMemoryServer_CreateNote_Validation(t *testing.T) {
	cases := []struct {
		name string
		req  *emotionmemory.CreateNoteRequest
		code codes.Code
	}{
		{"缺少 user_id", &emotionmemory.CreateNoteRequest{UserId: 0, Content: "x"}, codes.InvalidArgument},
		{"空内容", &emotionmemory.CreateNoteRequest{UserId: 1, Content: "   "}, codes.InvalidArgument},
		{"超长", &emotionmemory.CreateNoteRequest{UserId: 1, Content: strings.Repeat("记", memoryNoteMaxRunes+1)}, codes.InvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newMemoryServer(repository.NewInMemoryMemoryFactRepo(), nil)
			_, err := srv.CreateNote(context.Background(), c.req)
			if status.Code(err) != c.code {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), c.code, err)
			}
		})
	}
}

func TestMemoryServer_CreateNote_AsyncFailureDoesNotFailRequest(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	done := make(chan struct{})
	srv := newMemoryServer(repo, func(ctx context.Context, note *model.MemoryNote) error {
		close(done)
		return errors.New("llm down")
	})

	resp, err := srv.CreateNote(context.Background(), &emotionmemory.CreateNoteRequest{UserId: 1, Content: "x"})
	if err != nil {
		t.Fatalf("异步抽取失败不得影响回执: %v", err)
	}
	if resp.AckText != MemoryNoteAckText {
		t.Errorf("ack = %q", resp.AckText)
	}
	<-done
}

func TestMemoryServer_GetGraph_RequiresUser(t *testing.T) {
	srv := newMemoryServer(repository.NewInMemoryMemoryFactRepo(), nil)
	_, err := srv.GetGraph(context.Background(), &emotionmemory.GetGraphRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestMemoryServer_GetGraph_ReturnsUserFactsOnly(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryMemoryFactRepo()
	// 用户 1：一条事实；用户 2：一条事实
	_, err := repo.ApplyFacts(ctx, repository.PlanResult{Insert: []model.MemoryFact{
		{UserID: 1, Subject: "用户", Predicate: "职业", Object: "工程师", FactText: "u1", ValidFrom: time.Now().UTC(), Status: model.MemoryFactStatusActive},
		{UserID: 2, Subject: "用户", Predicate: "职业", Object: "设计师", FactText: "u2", ValidFrom: time.Now().UTC(), Status: model.MemoryFactStatusActive},
	}})
	if err != nil {
		t.Fatal(err)
	}

	srv := newMemoryServer(repo, nil)
	g, err := srv.GetGraph(ctx, &emotionmemory.GetGraphRequest{UserId: 1})
	if err != nil {
		t.Fatalf("GetGraph error: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("只能返回用户 1 的事实，got %d edges", len(g.Edges))
	}
	if g.Edges[0].Target != "entity:工程师" {
		t.Errorf("edge target = %q, 越到了别的用户？", g.Edges[0].Target)
	}
}
