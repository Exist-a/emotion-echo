// Package handler — survey_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.42 RED: survey handler 契约测试
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAssessmentClient 实现 downstream.AssessmentClient
type fakeAssessmentClient struct {
	items    []downstream.SurveyItem
	total    int
	detail   *downstream.SurveyDetail
	submit   *downstream.SubmitSurveyResp
	results  []downstream.SurveyResultItem
	rTotal   int
	result   *downstream.SurveyResultDetail
	err      error
}

func (f *fakeAssessmentClient) ListSurveys(_ context.Context, _ int) ([]downstream.SurveyItem, int, error) {
	return f.items, f.total, f.err
}
func (f *fakeAssessmentClient) GetSurvey(_ context.Context, _ uint64) (*downstream.SurveyDetail, error) {
	return f.detail, f.err
}
func (f *fakeAssessmentClient) SubmitSurvey(_ context.Context, _ uint64, _ downstream.SubmitSurveyReq) (*downstream.SubmitSurveyResp, error) {
	return f.submit, f.err
}
func (f *fakeAssessmentClient) ListResults(_ context.Context, _ int) ([]downstream.SurveyResultItem, int, error) {
	return f.results, f.rTotal, f.err
}
func (f *fakeAssessmentClient) GetResult(_ context.Context, _ uint64) (*downstream.SurveyResultDetail, error) {
	return f.result, f.err
}

func newSurveyRouter(client downstream.AssessmentClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&SurveyHandler{assessment: client}).Register(r)
	return r
}

func TestSurveyHandler_ListSurveys_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{
		items: []downstream.SurveyItem{{ID: 1, Code: "SDS", Title: "抑郁量表"}},
		total: 1,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys?limit=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"items"`)
	assert.Contains(t, w.Body.String(), `"SDS"`)
}

// TestSurveyHandler_ListSurveys_KeepsDescription
//
// E2E-14 实测评量卡片描述恒为空（3 个量表全空），根因：proto `SurveyItem`
// （agent.proto）无 description 字段，列表走 gRPC 时被静默丢弃。
// E2E-31（2026-10-08）已为 `SurveyItem` / `Survey` 补 `description` ⇒ 现在
// **gRPC 路径也必须透传描述**。
//
// 本用例取代原 `TestSurveyHandler_ListSurveys_HTTPSourceKeepsDescription`：
// 后者靠"HTTP 旁路"实现透传，而该旁路（assessmentBase 恒真）已随 E2E-31 删除。
// 断言口径不变（描述/分类/题数三项），只是被测路径从"旁路"换成"唯一路径"。
func TestSurveyHandler_ListSurveys_KeepsDescription(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{
		items: []downstream.SurveyItem{{
			ID: 7, Code: "BIG5", Title: "人格五因素量表",
			Description: "评估五大人格特质", Category: "personality", QuestionNum: 30, Version: 1,
		}},
		total: 1,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys?limit=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"description":"评估五大人格特质"`, "描述必须透传（历史 gRPC 路径会丢弃）")
	assert.Contains(t, body, `"category":"personality"`)
	assert.Contains(t, body, `"questionNum":30`)
}

func TestSurveyHandler_GetSurvey_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{detail: &downstream.SurveyDetail{
		ID: 1, Code: "SDS", Title: "抑郁量表", Category: "抑郁", Version: 1,
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"SDS"`)
}

func TestSurveyHandler_SubmitSurvey_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{submit: &downstream.SubmitSurveyResp{
		ResultID: 9, SurveyID: 1, TotalScore: 42.5, Answered: 20, RiskLevel: "moderate",
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit",
		bytes.NewReader([]byte(`{"answers":{"q1":3,"q2":2}}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"moderate"`)
}

func TestSurveyHandler_SubmitSurvey_NoAnswers_Returns400(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit",
		bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "answers is required")
}

func TestSurveyHandler_ListResults_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{
		results: []downstream.SurveyResultItem{{ResultID: 9, SurveyID: 1, RiskLevel: "low"}},
		rTotal:  1,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results?limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"low"`)
}

func TestSurveyHandler_GetResult_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{result: &downstream.SurveyResultDetail{
		ResultID: 9, SurveyID: 1, UserID: 7, TotalScore: 42.5, RiskLevel: "moderate",
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results/9", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"userId":7`)
}

func TestSurveyHandler_GetSurvey_NotFound_Returns404(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{err: &downstream.APIError{StatusCode: http.StatusNotFound, Msg: "survey not found"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "survey not found")
}

// ==================== HTTP 旁路用例的去向（E2E-31）====================
//
// 原文件此处有 5 个针对"HTTP 旁路"的用例（newSurveyRouterWithHTTP /
// GetSurveyHTTP_QuestionsAsArray / SubmitSurveyHTTP_PreservesAnswerKeys /
// GetResultHTTP_ReturnsRiskLevel / GetSurveyHTTP_QuestionsSortedByKey）。
// 它们测的是 `assessmentBase != ""` 恒真分支 —— 该分支已随 E2E-31 删除
// （见本文件顶部说明），故用例随之移除。
//
// **意图已迁移，不是丢弃**：
//   · 「questions 为有序数组 + id 来自 JSONB 键 + options 为 {id,text,score} 对象」
//     → `internal/downstream/assessment_test.go`（HTTP transport，含逆序输入验证排序）
//     → `internal/downstream/assessment_grpc_test.go`（gRPC transport，转换层）
//   · 「answers 键保真 "q1"（不得数值化）」→ 同上 gRPC 用例（服务端侧收到的键）
//     + `emotion-echo-shared/pkg/emotionassessment/agent_contract_test.go`（序列化往返）
//   · 「description 必须透传」→ 见上方 TestSurveyHandler_ListSurveys_KeepsDescription
//   · 端到端（浏览器真实链路）→ E2E-31 §0.3 #8 的 IAB 截图 + Playwright 48/48

// TestSurveyHandler_NoHTTPBypass_RegressionNail —— E2E-31 回归钉（静态源扫描）
//
// 本包曾用 `if h.assessmentBase != "" { …HTTP… return }` 把 5 个 gRPC 分支**全部绕开**；
// 而 `assessmentBase` 由 `config.go` 的默认值恒非空 ⇒ 条件恒真 ⇒ BFF→assessment-svc
// 的 gRPC 通道零调用（账本 E2E-F-208，成因 ADR-2026-09-survey-http-bypass）。
//
// 该旁路已随 E2E-31 删除。本守卫扫描本文件所在包的源码，**禁止重新引入**
// ——把"常量恒真当开关用"这类错误钉在源码层，而不是指望下次集成测试恰好覆盖到。
func TestSurveyHandler_NoHTTPBypass_RegressionNail(t *testing.T) {
	// ① 结构体不得再有 HTTP 旁路字段
	typ := reflect.TypeOf(SurveyHandler{})
	fieldNames := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		fieldNames = append(fieldNames, typ.Field(i).Name)
	}
	for _, forbidden := range []string{"assessmentBase", "assessmentHTTP"} {
		assert.NotContains(t, fieldNames, forbidden,
			"SurveyHandler 不得再有 %q 字段 —— 它是恒真旁路的载体", forbidden)
	}

	// ② 非注释代码里不得再出现旁路标识符（注释中作为历史说明提及是允许的）
	src, err := os.ReadFile("survey_handler.go")
	require.NoError(t, err)
	for _, line := range strings.Split(string(src), "\n") {
		code := strings.TrimSpace(line)
		if code == "" || strings.HasPrefix(code, "//") {
			continue
		}
		for _, forbidden := range []string{"assessmentBase", "WithAssessmentBase", "assessmentHTTP"} {
			assert.NotContains(t, code, forbidden,
				"非注释代码不得出现 %q（行：%s）—— 恒真 HTTP 旁路会让 gRPC 通道再次变成死代码",
				forbidden, code)
		}
	}
}
