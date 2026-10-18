// Package handler — agent_handler_test.go
//
// CL-06 · Phase 4：BFF 角色档案端点（`/api/v1/agents`）契约。
//
// 这三个端点只是**透传**（真正的归属/上限/字段校验在 user-svc，见 agentlogic.go），
// 但透传也有三条必须钉住的纪律：
//   · **不装配即 503**（fail-closed：宁可不提供服务，也不要"看似可用但静默丢弃"）；
//   · 路径参数非法（非数字 / ≤0）**不发下游**（避免拿垃圾 id 去打 user-svc）；
//   · 请求体非法 **400**（不是 500、也不是"用零值建一条空角色"）。
package handler

import (
	"bytes"
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

func newAgentRouter(a downstream.AgentProfileClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewAgentHandler(a).Register(r)
	return r
}

func TestAgentHandler_List_ReturnsProfiles(t *testing.T) {
	ac := &fakeAgentClient{profiles: []downstream.AgentProfile{
		{ID: 7, Name: "小E", IsDefault: true},
		{ID: 9, Name: "阿哲", Voice: "FunAudioLLM/CosyVoice2-0.5B:alex"},
	}}
	r := newAgentRouter(ac)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"code":0`)
	assert.Contains(t, body, `"agents"`)
	assert.Contains(t, body, `"name":"小E"`)
	assert.Contains(t, body, `"name":"阿哲"`)
	assert.Contains(t, body, `"total":2`)
	assert.NotContains(t, body, `"userId"`, "不得把内部 userId 回给前端（该值无用途）")
}

func TestAgentHandler_List_UpstreamError_Mapped(t *testing.T) {
	r := newAgentRouter(&fakeAgentClient{listErr: errors.New("down")})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestAgentHandler_Create_PassesFieldsAndReturnsSaved(t *testing.T) {
	ac := &fakeAgentClient{upsertResp: &downstream.AgentProfile{ID: 11, Name: "阿哲"}}
	r := newAgentRouter(ac)

	body := `{"name":"阿哲","gender":"male","persona":"直率的朋友","tone":"直接","boundaries":["不评价外貌"],"voice":"FunAudioLLM/CosyVoice2-0.5B:alex"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, ac.gotUpserts, 1)
	assert.Equal(t, int64(0), ac.gotUpserts[0].ID, "未带 id ⇒ 新建")
	assert.Equal(t, "阿哲", ac.gotUpserts[0].Name)
	assert.Equal(t, []string{"不评价外貌"}, ac.gotUpserts[0].Boundaries)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:alex", ac.gotUpserts[0].Voice)
	assert.Contains(t, w.Body.String(), `"id":11`)
}

func TestAgentHandler_Update_PassesID(t *testing.T) {
	ac := &fakeAgentClient{upsertResp: &downstream.AgentProfile{ID: 11, Name: "阿哲2"}}
	r := newAgentRouter(ac)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader([]byte(`{"id":11,"name":"阿哲2"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, ac.gotUpserts, 1)
	assert.Equal(t, int64(11), ac.gotUpserts[0].ID)
}

func TestAgentHandler_Upsert_InvalidBody_400(t *testing.T) {
	ac := &fakeAgentClient{}
	r := newAgentRouter(ac)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader([]byte(`{not json`)))
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, ac.gotUpserts, "非法体不得下达下游（否则会用零值建出一条空角色）")
}

func TestAgentHandler_Upsert_UpstreamValidationError_Mapped(t *testing.T) {
	// user-svc 侧字段/上限校验失败 → InvalidArgument → 400（不是 502）
	ac := &fakeAgentClient{upsertErr: &downstream.APIError{StatusCode: http.StatusBadRequest, Msg: "name too long"}}
	r := newAgentRouter(ac)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader([]byte(`{"name":"x"}`)))
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "name too long")
}

func TestAgentHandler_Delete_ById(t *testing.T) {
	ac := &fakeAgentClient{}
	r := newAgentRouter(ac)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/agents/11", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []int64{11}, ac.gotDeletes)
	assert.Contains(t, w.Body.String(), `"success":true`)
}

func TestAgentHandler_Delete_InvalidID_400(t *testing.T) {
	ac := &fakeAgentClient{}
	r := newAgentRouter(ac)

	for _, path := range []string{"/api/v1/agents/abc", "/api/v1/agents/0", "/api/v1/agents/-3"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, path, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, "path=%s", path)
	}
	assert.Empty(t, ac.gotDeletes)
}

func TestAgentHandler_Delete_UpstreamError_Mapped(t *testing.T) {
	ac := &fakeAgentClient{deleteErr: errors.New("down")}
	r := newAgentRouter(ac)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/agents/11", nil))

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

// 未装配 ⇒ 三个端点都 503（fail-closed：不要"看起来能点但什么都不发生"）
func TestAgentHandler_NotWired_FailClosed(t *testing.T) {
	r := newAgentRouter(nil)

	cases := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/agents", ""},
		{http.MethodPost, "/api/v1/agents", `{"name":"x"}`},
		{http.MethodDelete, "/api/v1/agents/11", ""},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		var req *http.Request
		if c.body != "" {
			req = httptest.NewRequest(c.method, c.path, bytes.NewReader([]byte(c.body)))
		} else {
			req = httptest.NewRequest(c.method, c.path, nil)
		}
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, "%s %s", c.method, c.path)
	}
}

// 编译期：downstream.AgentProfileClient 满足 handler 需要的能力（防签名漂移）
func TestAgentHandler_ClientInterfaceShape(t *testing.T) {
	var _ interface {
		ListAgentProfiles(ctx context.Context) ([]downstream.AgentProfile, error)
		UpsertAgentProfile(ctx context.Context, req downstream.AgentProfileUpsertReq) (*downstream.AgentProfile, error)
		DeleteAgentProfile(ctx context.Context, agentID int64) error
	} = (*fakeAgentClient)(nil)
}
