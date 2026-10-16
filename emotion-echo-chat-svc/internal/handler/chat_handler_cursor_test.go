package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-chat-svc/internal/model"
)

// CL-05 Phase 6 · T-7：chat-svc 的 **HTTP 降级路径**（`*_TRANSPORT=http` 回滚开关）
// 必须与 gRPC 主路径一样认 `cursor`。
//
// 现状（2026-10-14 亲核）：`ListMessagesHandler` 只解析 `id` 与 `limit`
// （internal/handler/chat_handler.go:84-99），**完全忽略 cursor** ⇒ 一旦把
// `CHAT_TRANSPORT` 切到 http，翻页会**静默失效**：永远返回最新一页（用户上滚看不到历史）。
// gRPC 主路径是好的，所以这个洞只在"切回滚开关"时才显形 —— 正是最容易被漏掉的那类。
//
// 钉法：直接经仓储塞 3 条消息 → `?limit=2` 取最新 2 条，取该页最小 id 作 cursor →
// 再带 cursor 请求，**断言更旧的一页为空**（3 条里最小的已是最早 ⇒ 没有更旧的）。
// 若 cursor 被忽略，第二次会**又返回最新 2 条**（非空）⇒ 本测试红。
func TestChatHandler_ListMessages_HonorsCursor(t *testing.T) {
	svcCtx := newTestSVC()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/conversations", CreateConversationHandler(svcCtx))
	r.GET("/api/v1/conversations/:id/messages", ListMessagesHandler(svcCtx))

	// 1) 建会话（走 HTTP，与既有 e2e 用例同形）
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/conversations",
		bytes.NewBufferString(`{"title":"cursor","userId":42}`))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate = reqWithUser(reqCreate, 42)
	recCreate := httptest.NewRecorder()
	r.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusOK, recCreate.Code, recCreate.Body.String())

	var convBody map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &convBody))
	var convID int64
	if v, ok := convBody["id"].(float64); ok {
		convID = int64(v)
	} else if conv, ok := convBody["conversation"].(map[string]any); ok {
		if v, ok := conv["id"].(float64); ok {
			convID = int64(v)
		}
	}
	require.NotZero(t, convID, "拿不到会话 id，body=%s", recCreate.Body.String())

	// 2) 直接经仓储塞 3 条（避开 HTTP 发送路径的鉴权差异，让本钉只测分页契约）
	for i := 1; i <= 3; i++ {
		m := &model.Message{
			ConversationID: convID,
			UserID:         42,
			Role:           "user",
			Content:        fmt.Sprintf("m%d", i),
		}
		require.NoError(t, svcCtx.ConversationRepo.AppendMessage(context.Background(), m))
	}

	route := "/api/v1/conversations/" + intToStr(convID) + "/messages"
	list := func(query string) []map[string]any {
		req := httptest.NewRequest(http.MethodGet, route+query, nil)
		req = reqWithUser(req, 42)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body.Messages
	}

	page1 := list("?limit=2")
	require.Len(t, page1, 2, "首屏应给最新 2 条")

	minID := int64(page1[0]["id"].(float64))
	for _, m := range page1 {
		if id := int64(m["id"].(float64)); id < minID {
			minID = id
		}
	}

	// 3) 带 cursor 翻更旧的一页：3 条里比 minID 更旧的正好还有 1 条
	page2 := list("?limit=2&cursor=" + intToStr(minID))
	require.Len(t, page2, 1, "带 cursor 应只返回比 cursor 更旧的那 1 条")
	for _, m := range page2 {
		assert.Less(t, int64(m["id"].(float64)), minID,
			"带 cursor 时只能返回 id < cursor 的消息；忽略 cursor 会又返回最新一页（id ≥ cursor）")
	}
}
