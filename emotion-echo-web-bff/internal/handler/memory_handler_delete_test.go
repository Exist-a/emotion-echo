// CL-08d：删除端点的 handler 契约（fake client）。
package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMemoryClientForDelete struct {
	downstream.MemoryClient
	listErr    error
	deleteErr  error
	delScope   string
	delID      int64
	delUser    int64
	listResult []downstream.MemoryItem
}

func (f *fakeMemoryClientForDelete) ListMemory(ctx context.Context, userID int64) ([]downstream.MemoryItem, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeMemoryClientForDelete) DeleteMemory(ctx context.Context, userID int64, scope string, id int64) (string, error) {
	if f.deleteErr != nil {
		return "", f.deleteErr
	}
	uid, _ := downstream.UserIDFromContext(ctx)
	f.delUser = uid
	f.delScope = scope
	f.delID = id
	return "", nil
}

func deleteRouter(client downstream.MemoryClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewMemoryHandler(client)
	r.GET("/api/v1/memory/items", h.ListMemory)
	r.DELETE("/api/v1/memory/facts/:id", func(c *gin.Context) { h.DeleteMemory(c, "fact") })
	r.DELETE("/api/v1/memory/notes/:id", func(c *gin.Context) { h.DeleteMemory(c, "note") })
	r.DELETE("/api/v1/memory", func(c *gin.Context) { h.DeleteMemory(c, "all") })
	// 身份注入（网关同型）：测试用中间件模拟
	r.Use(func(c *gin.Context) { c.Next() })
	return r
}

// 直接以带身份 ctx 调 handler 的单测走 fake 源；路由层测试用 WithUserID 中间件。
func TestMemoryDelete_ScopeAndIDPassThrough(t *testing.T) {
	client := &fakeMemoryClientForDelete{}
	h := NewMemoryHandler(client)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	req, _ := http.NewRequest(http.MethodDelete, "/", nil)
	c.Request = req
	c.Request = c.Request.WithContext(downstream.WithUserID(context.Background(), 7))
	h.DeleteMemory(c, "fact")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "fact", client.delScope)
	assert.EqualValues(t, 42, client.delID)
	assert.EqualValues(t, 7, client.delUser, "身份必须来自 ctx（不收客户端传参）")
}

func TestMemoryDelete_InvalidID(t *testing.T) {
	h := NewMemoryHandler(&fakeMemoryClientForDelete{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "0"}}
	c.Request, _ = http.NewRequest(http.MethodDelete, "/", nil)
	c.Request = c.Request.WithContext(downstream.WithUserID(context.Background(), 7))
	h.DeleteMemory(c, "note")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMemoryDelete_DownstreamError(t *testing.T) {
	h := NewMemoryHandler(&fakeMemoryClientForDelete{deleteErr: errors.New("down")})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "5"}}
	c.Request, _ = http.NewRequest(http.MethodDelete, "/", nil)
	c.Request = c.Request.WithContext(downstream.WithUserID(context.Background(), 7))
	h.DeleteMemory(c, "fact")
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestMemoryList_PassesThroughItems(t *testing.T) {
	h := NewMemoryHandler(&fakeMemoryClientForDelete{listResult: []downstream.MemoryItem{
		{Kind: "fact", ID: 1, Text: "用户居住地杭州", SourceKind: "message", SourceID: 100, CreatedAt: "2026-10-25"},
		{Kind: "note", ID: 2, Text: "我在学萨克斯", CreatedAt: "2026-10-25"},
	}})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Request = c.Request.WithContext(downstream.WithUserID(context.Background(), 7))
	h.ListMemory(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "用户居住地杭州")
	assert.Contains(t, w.Body.String(), "我在学萨克斯")
}

func TestMemoryDelete_NoClient(t *testing.T) {
	h := NewMemoryHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request, _ = http.NewRequest(http.MethodDelete, "/", nil)
	c.Request = c.Request.WithContext(downstream.WithUserID(context.Background(), 7))
	h.DeleteMemory(c, "fact")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
