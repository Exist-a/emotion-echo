// Package handler — memory_handler_test.go
//
// CL-08a · A5：「我的记忆」端点契约（假客户端，不起 gRPC）。

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMemoryClient struct {
	note       *downstream.MemoryNoteResult
	graph      *downstream.MemoryGraph
	err        error
	gotUserID  int64
	gotContent string
}

func (f *fakeMemoryClient) CreateNote(ctx context.Context, userID int64, content string) (*downstream.MemoryNoteResult, error) {
	f.gotUserID, f.gotContent = userID, content
	if f.err != nil {
		return nil, f.err
	}
	return f.note, nil
}

func (f *fakeMemoryClient) GetGraph(ctx context.Context, userID int64) (*downstream.MemoryGraph, error) {
	f.gotUserID = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.graph, nil
}

func newMemoryTestRouter(h *MemoryHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/memory/notes", h.CreateNote)
	r.GET("/api/v1/memory/graph", h.GetGraph)
	return r
}

func TestMemoryHandler_CreateNote_HappyPath(t *testing.T) {
	fake := &fakeMemoryClient{note: &downstream.MemoryNoteResult{NoteID: 9, AckText: "记忆已更新"}}
	r := newMemoryTestRouter(NewMemoryHandler(fake))

	body, _ := json.Marshal(map[string]string{"content": "  前年去了西藏  "})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memory/notes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			NoteID  int64  `json:"noteId"`
			AckText string `json:"ackText"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, int64(9), resp.Data.NoteID)
	assert.Equal(t, "记忆已更新", resp.Data.AckText)
	assert.Equal(t, int64(7), fake.gotUserID, "user_id 必须来自鉴权上下文")
	assert.Equal(t, "前年去了西藏", fake.gotContent, "内容应 trim 后下发")
}

func TestMemoryHandler_CreateNote_ValidationAndAuth(t *testing.T) {
	cases := []struct {
		name    string
		handler *MemoryHandler
		header  string
		body    string
		want    int
	}{
		{"无鉴权头", NewMemoryHandler(&fakeMemoryClient{}), "", `{"content":"x"}`, http.StatusUnauthorized},
		{"空内容", NewMemoryHandler(&fakeMemoryClient{}), "7", `{"content":"   "}`, http.StatusBadRequest},
		{"非法 JSON", NewMemoryHandler(&fakeMemoryClient{}), "7", `{`, http.StatusBadRequest},
		{"未装配客户端", NewMemoryHandler(nil), "7", `{"content":"x"}`, http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newMemoryTestRouter(c.handler)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/memory/notes", bytes.NewReader([]byte(c.body)))
			req.Header.Set("Content-Type", "application/json")
			if c.header != "" {
				req.Header.Set("X-User-Id", c.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, c.want, w.Code, w.Body.String())
		})
	}
}

func TestMemoryHandler_GetGraph(t *testing.T) {
	fake := &fakeMemoryClient{graph: &downstream.MemoryGraph{
		Nodes: []downstream.MemoryGraphNode{{ID: "user:7", Label: "我", Kind: "user", Weight: 1}},
		Edges: []downstream.MemoryGraphEdge{{Source: "user:7", Target: "entity:工程师", Label: "职业"}},
	}}
	r := newMemoryTestRouter(NewMemoryHandler(fake))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memory/graph", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int                       `json:"code"`
		Data downstream.MemoryGraph    `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Nodes, 1)
	require.Len(t, resp.Data.Edges, 1)
	assert.Equal(t, int64(7), fake.gotUserID)
}

func TestMemoryHandler_GetGraph_NilClient(t *testing.T) {
	r := newMemoryTestRouter(NewMemoryHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memory/graph", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// CL-08d：接口扩展后的空实现（既有 fake 不动）。
func (f *fakeMemoryClient) ListMemory(ctx context.Context, userID int64) ([]downstream.MemoryItem, error) {
	return nil, nil
}

func (f *fakeMemoryClient) DeleteMemory(ctx context.Context, userID int64, scope string, id int64) (string, error) {
	return "", nil
}
