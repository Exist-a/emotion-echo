package logic

import (
	"context"
	"os"
	"strings"
	"testing"

	"emotion-echo-chat-svc/internal/events"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ctxWithUserID + newTestCtx are shared helpers for all logic/*_test.go
// files in this package. Defined here (the older sibling) so existing
// imports keep working; sendmessagelogic_test.go reuses them.
func ctxWithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, sharedmw.CtxUserIDKey{}, uid)
}

// 测试公用：构造一个完整测试上下文
func newTestCtx(t *testing.T) (*svc.ServiceContext, *repository.InMemoryConversationRepo, *events.InMemoryEventPublisher) {
	t.Helper()
	repo := repository.NewInMemoryConversationRepo()
	pub := events.NewInMemoryEventPublisher()
	svcCtx := &svc.ServiceContext{
		ConversationRepo: repo,
		EventPublisher:   pub,
	}
	return svcCtx, repo, pub
}

func TestCreateConversationLogic_WithTitle_PublishesEvent(t *testing.T) {
	t.Parallel()

	svcCtx, _, pub := newTestCtx(t)
	l := NewCreateConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.CreateConversation(&types.CreateConversationReq{Title: "今晚的咨询"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(100), resp.Conversation.UserId)
	assert.Equal(t, "今晚的咨询", resp.Conversation.Title)

	// 断言：发布了 conversation.created 事件
	evts := pub.Events(events.TopicChatEvents)
	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeConversationCreated, evts[0].Type)
	assert.Equal(t, "chat-svc", evts[0].Source)
}

func TestCreateConversationLogic_EmptyTitle_DefaultsToEmpty(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	l := NewCreateConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.CreateConversation(&types.CreateConversationReq{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "", resp.Conversation.Title)
}

func TestCreateConversationLogic_NoUserID_Returns401(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	// 不塞 userID
	l := NewCreateConversationLogic(context.Background(), svcCtx)

	resp, err := l.CreateConversation(&types.CreateConversationReq{Title: "x"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

// =====================================================
// Stage 94 PR-7 §P0-8 · persistWithOutbox 退化路径数据完整性
// =====================================================
//
// code-review-2026-09-14 §P0-8 原文:chat-svc `persistWithOutbox` 路径 2/3:
// DB 写完但 outbox 写失败 → 事件静默丢失(1-2d)。
//
// 旧实现退化路径(createconversationlogic.go:126-143):
//
//	// 业务表持久化（路径 2/3 共用）
//	if err := l.svcCtx.ConversationRepo.CreateConversation(l.ctx, conv); err != nil {
//	    return err
//	}
//	...
//	if l.svcCtx.OutboxRepo != nil {
//	    return l.svcCtx.OutboxRepo.CreateInTx(nil, &repository.OutboxEvent{...})
//	}
//
// 风险:业务表 CreateConversation 成功后(已落库,ID 自增)→ outbox CreateInTx 失败
// (DB 抖动 / 唯一约束违反 / outbox schema 未应用)→ 业务记录存在但事件丢失
// (consumer 永远收不到消息,metrics/dashboard 空)。
//
// 修复目标(§3.5 路径 B):业务写 + outbox 写必须在同一 DB Transaction 内,
// 任意一步失败 → 全部回滚 → 业务行不存在 + outbox 行不存在 → 一致性保证。
//
// 测试策略:源码字面量断言钉死契约。
//   1) 反向断言:createconversationlogic.go 不应出现"业务 CreateConversation 成功
//      后单独调 CreateInTx(nil, ...)"模式(无事务独立写)
//   2) 正向断言:必须用 gorm.DB.Transaction(...) 包裹业务 + outbox
//   3) sendmessagelogic.go 同源 fix 同步(AppendMessage + CreateInTx 应同事务)

// TestCreateConversationLogic_PersistWithOutbox_AtomicTransaction §P0-8 字面量断言:
//
// createconversationlogic.go 业务表持久化 + outbox 写必须满足:
//
//   - DB 齐备:同事务内 CreateConversationTx + CreateInTx(tx, ...)(路径 1)
//   - DB nil / OutboxRepo nil:仅业务写 + best-effort Publish,无"拆开写"黑洞
//   - 绝对禁止:DB 齐备时业务 CreateConversation(非 Tx 版) + 独立 CreateInTx
//
// 这是 §P0-8 bug 的根本模式:业务先落库 → outbox 写失败 → 事件静默丢失。
func TestCreateConversationLogic_PersistWithOutbox_AtomicTransaction(t *testing.T) {
	srcBytes, err := os.ReadFile("createconversationlogic.go")
	if err != nil {
		t.Skipf("cannot read createconversationlogic.go: %v", err)
	}
	// 只看代码本体,不看注释(避免 self-referential 误命中)
	src := stripGoComments(string(srcBytes))

	// 反向断言(§P0-8 bug 模式):不应出现 CreateInTx(nil, ...)
	// —— 传 nil tx 是 PostgresOutboxRepo 下 panic 模式,业务写完但 outbox 写失败 = 事件丢失。
	if strings.Contains(src, "CreateInTx(nil,") {
		t.Errorf("createconversationlogic.go 仍含 CreateInTx(nil, ...) —— §P0-8 修复要求\n"+
			"业务写 + outbox 写必须在同一 DB.Transaction 内,不允许传 nil tx(退化路径下\n"+
			"业务写完但 outbox 写失败 → 事件静默丢失)")
	}

	// 正向断言:DB.Transaction 包业务 + outbox
	mustContain := []string{
		`CreateConversationTx(tx,`,
		`OutboxRepo.CreateInTx(tx,`,
		// DB.Transaction 是事务化路径的入口
		`l.svcCtx.DB.Transaction(`,
	}
	for _, m := range mustContain {
		if !strings.Contains(src, m) {
			t.Errorf("createconversationlogic.go 缺 %q —— §P0-8 修复要求业务+outbox 同事务", m)
		}
	}
}

// TestSendMessageLogic_PersistWithOutbox_AtomicTransaction §P0-8 字面量断言:
//
// sendmessagelogic.go 同 bug——DB 齐备路径必须用 AppendMessageTx + CreateInTx(tx, ...)
// 同一事务;DB nil 时允许非 Tx 版 AppendMessage(无 outbox 黑洞)。
func TestSendMessageLogic_PersistWithOutbox_AtomicTransaction(t *testing.T) {
	srcBytes, err := os.ReadFile("sendmessagelogic.go")
	if err != nil {
		t.Skipf("cannot read sendmessagelogic.go: %v", err)
	}
	src := stripGoComments(string(srcBytes))

	if strings.Contains(src, "CreateInTx(nil,") {
		t.Errorf("sendmessagelogic.go 仍含 CreateInTx(nil, ...) —— §P0-8 修复要求\n"+
			"AppendMessage + outbox 写必须在同一 DB.Transaction 内")
	}

	// 正向断言:DB 齐备路径必须用 AppendMessageTx(同事务) + CreateInTx(tx, ...)
	mustContain := []string{
		`AppendMessageTx(tx,`,
		`OutboxRepo.CreateInTx(tx,`,
		`l.svcCtx.DB.Transaction(`,
	}
	for _, m := range mustContain {
		if !strings.Contains(src, m) {
			t.Errorf("sendmessagelogic.go 缺 %q —— §P0-8 修复要求业务+outbox 同事务", m)
		}
	}
}

// stripGoComments 剥离 Go 行注释 + 块注释 + 字符串字面量,简化版
// 仅用于字面量断言避免 self-referential 误命中。
func stripGoComments(src string) string {
	var out strings.Builder
	inBlock := false
	inStr := false
	inRawStr := false // 反引号
	prevQuote := byte(0)
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inBlock {
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if inRawStr {
			if c == '`' {
				inRawStr = false
			}
			out.WriteByte(c)
			continue
		}
		if inStr {
			if c == '\\' && i+1 < len(src) {
				out.WriteByte(c)
				out.WriteByte(src[i+1])
				i++
				continue
			}
			if c == prevQuote {
				inStr = false
			}
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			inBlock = true
			i++
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			// 行注释到 \n
			for i < len(src) && src[i] != '\n' {
				i++
			}
			out.WriteByte('\n') // 保留换行
			continue
		}
		if c == '`' {
			inRawStr = true
			out.WriteByte(c)
			continue
		}
		if c == '"' || c == '\'' {
			inStr = true
			prevQuote = c
			out.WriteByte(c)
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}