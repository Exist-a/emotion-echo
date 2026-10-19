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
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// authCtx 模拟 gRPC 拦截器写入的可信身份（x-user-id metadata）。
func authCtx(uid int64) context.Context {
	return context.WithValue(context.Background(), grpcinterceptor.CtxUserIDKeyType{}, uid)
}

func TestMemoryServer_CreateNote_Success(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	called := make(chan int64, 1)
	srv := newMemoryServer(repo, func(ctx context.Context, note *model.MemoryNote) error {
		called <- note.ID
		return nil
	})

	resp, err := srv.CreateNote(authCtx(3), &emotionmemory.CreateNoteRequest{
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
		// 注：body user_id 为空是合法的（以 metadata 身份为准）；身份缺失由 Unauthenticated 测试覆盖。
		{"空内容", &emotionmemory.CreateNoteRequest{UserId: 1, Content: "   "}, codes.InvalidArgument},
		{"超长", &emotionmemory.CreateNoteRequest{UserId: 1, Content: strings.Repeat("记", memoryNoteMaxRunes+1)}, codes.InvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newMemoryServer(repository.NewInMemoryMemoryFactRepo(), nil)
			_, err := srv.CreateNote(authCtx(1), c.req)
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

	resp, err := srv.CreateNote(authCtx(1), &emotionmemory.CreateNoteRequest{UserId: 1, Content: "x"})
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
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("无身份必须 Unauthenticated（fail-closed），got %v", status.Code(err))
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
	g, err := srv.GetGraph(authCtx(1), &emotionmemory.GetGraphRequest{UserId: 1})
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

func TestMemoryServer_UserIDMismatchDenied(t *testing.T) {
	// 第二方核对抓出的越权路径：请求体 user_id 必须与 metadata 身份一致，
	// 否则本机任意进程可用 x-user-id:1 + body user_id:2 读写他人的记忆。
	srv := newMemoryServer(repository.NewInMemoryMemoryFactRepo(), nil)

	_, err := srv.CreateNote(authCtx(1), &emotionmemory.CreateNoteRequest{UserId: 2, Content: "x"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("CreateNote 写他人：code = %v, want PermissionDenied", status.Code(err))
	}

	_, err = srv.GetGraph(authCtx(1), &emotionmemory.GetGraphRequest{UserId: 2})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetGraph 读他人：code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestMemoryServer_BodyUserIDZeroUsesMetadata(t *testing.T) {
	repo := repository.NewInMemoryMemoryFactRepo()
	srv := newMemoryServer(repo, nil)
	resp, err := srv.CreateNote(authCtx(5), &emotionmemory.CreateNoteRequest{Content: "x"})
	if err != nil {
		t.Fatalf("body user_id 为空时应以 metadata 为准: %v", err)
	}
	note, _ := repo.GetNote(context.Background(), resp.NoteId)
	if note == nil || note.UserID != 5 {
		t.Fatalf("落库归属必须 = metadata 身份 5, got %+v", note)
	}
}
