// Package downstream — assessment_grpc_test.go
//
// E2E-31（2026-10-08，内部 RPC 收敛）：BFF → assessment-svc **gRPC 路径**的契约测试。
//
// 为什么必须补：改造前这条通道零调用（`assessmentBase != ""` 恒真把它全绕开），
// 所以它没有任何测试——`survey_handler_test.go` 里 5 个用例测的全是 HTTP 旁路。
// 旁路删除后 gRPC 成为唯一路径，本文件补上对**真实报言语义**的断言，重点是
// E2E-13 的两条历史根因：
//   ① 题目必须有序且键名保真（"q1"），选项必须是 {id,text,score} 对象；
//   ② 作答键不得被数值化（"q1" 不能变成 "1"）。
//
// 手法：用 `bufconn` 在进程内起真实 gRPC server（无网络、无端口），
// 与服务端侧的转换函数形成"两端夹逼"——中间任何一侧改坏都会被本文件抓到。

package downstream

import (
	"context"
	"net"
	"testing"

	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// stubAssessmentServer 只实现本文件用到的 RPC
type stubAssessmentServer struct {
	emotionassessment.UnimplementedAssessmentServiceServer
	lastSubmit *emotionassessment.SubmitSurveyRequest
	survey     *emotionassessment.Survey
	results    []*emotionassessment.SurveyResult
}

func (s *stubAssessmentServer) GetSurvey(context.Context, *emotionassessment.GetSurveyRequest) (*emotionassessment.Survey, error) {
	return s.survey, nil
}

func (s *stubAssessmentServer) SubmitSurvey(_ context.Context, req *emotionassessment.SubmitSurveyRequest) (*emotionassessment.SurveyResult, error) {
	s.lastSubmit = req
	return &emotionassessment.SurveyResult{
		ResultId:     9,
		SurveyId:     req.GetSurveyId(),
		TotalScore:   12.5, // 小数：验证不再被 int32 截断
		Answered:     2,
		RiskLevel:    "moderate",
		FactorScores: map[string]float64{"O": 12.5},
		ScoreKind:    "dimension_sum",
	}, nil
}

func (s *stubAssessmentServer) ListMyResults(context.Context, *emotionassessment.ListMyResultsRequest) (*emotionassessment.ListMyResultsResponse, error) {
	return &emotionassessment.ListMyResultsResponse{Items: s.results, Total: int32(len(s.results))}, nil
}

func (s *stubAssessmentServer) GetSurveyResult(context.Context, *emotionassessment.GetSurveyResultRequest) (*emotionassessment.SurveyResult, error) {
	return &emotionassessment.SurveyResult{
		ResultId:     9,
		SurveyId:     1,
		UserId:       7,
		TotalScore:   12.5,
		RiskLevel:    "moderate",
		DurationSec:  60,
		Answers:      map[string]int32{"q1": 1},
		SubmittedAt:  1789878019,
		FactorScores: map[string]float64{"O": 12.5},
		ScoreKind:    "dimension_sum",
	}, nil
}

// newGRPCClientForTest 起 in-process gRPC server 并返回真实 gRPC 客户端
func newGRPCClientForTest(t *testing.T, stub *stubAssessmentServer) AssessmentClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	emotionassessment.RegisterAssessmentServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	c := NewAssessmentGRPCClient(conn)
	require.NotNil(t, c, "Transport=grpc + 非 nil conn 必须返回 gRPC 实现")
	return c
}

// TestAssessmentGRPC_GetSurvey_OrderedKeysAndStructuredOptions —— E2E-31 #1/#4
func TestAssessmentGRPC_GetSurvey_OrderedKeysAndStructuredOptions(t *testing.T) {
	stub := &stubAssessmentServer{survey: &emotionassessment.Survey{
		Id: 1, Code: "PHQ-9", Title: "抑郁症筛查量表", Description: "过去两周内…",
		Questions: []*emotionassessment.SurveyQuestion{
			{Key: "q1", Order: 1, Title: "做事时提不起劲或没有兴趣", QuestionType: "radio",
				OptionItems: []*emotionassessment.SurveyOption{
					{Id: 1, Text: "完全没有", Score: 0},
					{Id: 4, Text: "几乎每天", Score: 3},
				}},
			{Key: "q2", Order: 2, Title: "感到心情低落", QuestionType: "radio",
				OptionItems: []*emotionassessment.SurveyOption{{Id: 1, Text: "完全没有", Score: 0}}},
		},
	}}
	c := newGRPCClientForTest(t, stub)

	s, err := c.GetSurvey(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "过去两周内…", s.Description, "description 曾在 proto 缺失（E2E-14 描述全空）")

	require.Len(t, s.Questions, 2)
	// 键名保真（不是按切片下标重编的 q0/q1）
	assert.Equal(t, "q1", s.Questions[0]["id"])
	assert.Equal(t, "q2", s.Questions[1]["id"])
	assert.Equal(t, "做事时提不起劲或没有兴趣", s.Questions[0]["title"])

	// 选项必须是对象数组（前端取 opt.id / opt.text / opt.score）
	opts, ok := s.Questions[0]["options"].([]map[string]any)
	require.True(t, ok, "options 应为 []map[string]any，实际 %T", s.Questions[0]["options"])
	require.Len(t, opts, 2)
	assert.Equal(t, "几乎每天", opts[1]["text"])
	assert.Equal(t, 3, opts[1]["score"], "计分取 score 而非 id（E2E-F-97）")
}

// TestAssessmentGRPC_SubmitSurvey_KeysStayQuestionKeys —— E2E-31 #3
func TestAssessmentGRPC_SubmitSurvey_KeysStayQuestionKeys(t *testing.T) {
	stub := &stubAssessmentServer{}
	c := newGRPCClientForTest(t, stub)

	resp, err := c.SubmitSurvey(context.Background(), 1, SubmitSurveyReq{
		Answers: map[string]int{"q1": 3, "q2": 0},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.NotNil(t, stub.lastSubmit, "服务端必须收到 SubmitSurvey")
	got := stub.lastSubmit.GetAnswers()
	require.Contains(t, got, "q1")
	assert.NotContains(t, got, "1", "键被数值化 —— E2E-13 根因复发（\"q1\" → 1 → \"1\"）")
	assert.Equal(t, int32(3), got["q1"])

	// 分数语义（E2E-31 #5）
	assert.Equal(t, 12.5, resp.TotalScore, "total_score int32 截断会丢小数")
	assert.Equal(t, "dimension_sum", resp.ScoreKind)
	assert.Equal(t, float64(12.5), resp.FactorScores["O"])
}

// TestAssessmentGRPC_ListResults_Implemented —— E2E-31 #11
//
// 该方法改造前是 `return fmt.Errorf("... not implemented ...")` 的 stub，
// 人格画像因此只能另建 HTTP 客户端取数。
func TestAssessmentGRPC_ListResults_Implemented(t *testing.T) {
	stub := &stubAssessmentServer{results: []*emotionassessment.SurveyResult{{
		ResultId: 9, SurveyId: 1, TotalScore: 12.5, RiskLevel: "dimension_profile",
		SubmittedAt: 1789878019, FactorScores: map[string]float64{"O": 12.5}, ScoreKind: "dimension_sum",
	}}}
	c := newGRPCClientForTest(t, stub)

	items, total, err := c.ListResults(context.Background(), 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, 12.5, items[0].TotalScore)
	assert.Equal(t, float64(12.5), items[0].FactorScores["O"], "画像取数依赖 factorScores")
	assert.Equal(t, "dimension_sum", items[0].ScoreKind)
	assert.Equal(t, int64(1789878019), items[0].SubmittedAt)
}

// TestAssessmentGRPC_GetResult_Implemented —— E2E-31 #11
func TestAssessmentGRPC_GetResult_Implemented(t *testing.T) {
	c := newGRPCClientForTest(t, &stubAssessmentServer{})

	r, err := c.GetResult(context.Background(), 9)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, int64(7), r.UserID)
	assert.Equal(t, 60, r.DurationSec)
	assert.Equal(t, 1, r.Answers["q1"], "结果侧作答键同样要保真（原 repeated Answer 会丢 \"q1\"）")
	assert.Equal(t, int64(1789878019), r.SubmittedAt)
	assert.Equal(t, "dimension_sum", r.ScoreKind)
}
