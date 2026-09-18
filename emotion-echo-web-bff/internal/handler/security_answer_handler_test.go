// Package handler — security_answer_handler_test.go
//
// R-01 #1 / E2E-F-61：BFF handler 层「密保校验」的回归钉。
//
// 为什么必须在本层钉：原缺陷（fail-open）就发生在**本层**——handler 用
// `h.user.Login(ctx, username, "dummy")` 当作"用户是否存在"的探针，而真实用户
// 的密码不是 "dummy"，于是恒走 `err != nil` 分支、恒返回 `success:true`，
// 答案从不被校验。当时全部密保测试都在 user-svc logic 层
// （`emotion-echo-user-svc/internal/logic/authlogic_test.go`），本层零覆盖，
// 因此该缺陷通过了所有测试。
//
// fake 的设计要点：它按「用户是否存在 + 答案是否正确」**真实作答**，
// 而不是恒返回一个固定的 error。若有人把实现改回"恒返回成功"，
// 本组用例必然判红（已用负向对照验证：把 handler 回退为 Login("dummy") 探测版本后，
// wrong-answer / unknown-user 两条用例即变红）。
package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// securityAnswerUserClient 是密保校验专用 fake。
//
// 与通用 fakeUserClient 的关键差别：后者的 VerifySecurityAnswerByUsername 是
// `return f.err`（恒定值），无法区分"答案错"与"答案对"，因此抓不出 fail-open；
// 本 fake 真的比对 username/answer。
type securityAnswerUserClient struct {
	fakeUserClient // 复用其余方法以满足 UserClient 接口
	knownUser      string
	knownAnswer    string
}

func (f *securityAnswerUserClient) VerifySecurityAnswerByUsername(_ context.Context, username string, _ int, answer string) error {
	if username != f.knownUser || answer != f.knownAnswer {
		// 用户不存在与答案错误返回同一种错误（防用户名枚举，与 user-svc 语义一致）
		return &downstream.APIError{StatusCode: http.StatusUnauthorized, Msg: "security answer verification failed"}
	}
	return nil
}

// TestAuthHandler_VerifySecurityAnswer_WrongAnswer_Returns401 是本组最核心的一条：
// 答案错误必须被拒。若实现回退为 fail-open（恒 success），本用例立即变红。
func TestAuthHandler_VerifySecurityAnswer_WrongAnswer_Returns401(t *testing.T) {
	router := newAuthRouter(t, &securityAnswerUserClient{knownUser: "alice", knownAnswer: "blue"})

	w := postJSON(router, "/api/v1/auth/verify-security-answer",
		`{"username":"alice","questionOrder":1,"answer":"WRONG"}`)

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"错误答案必须 401 —— 返回 200 即说明校验被绕过（fail-open 复发）")
	assert.NotContains(t, w.Body.String(), `"success":true`,
		"错误答案的响应体不得含 success:true")
}

func TestAuthHandler_VerifySecurityAnswer_UnknownUser_Returns401(t *testing.T) {
	router := newAuthRouter(t, &securityAnswerUserClient{knownUser: "alice", knownAnswer: "blue"})

	w := postJSON(router, "/api/v1/auth/verify-security-answer",
		`{"username":"nobody","questionOrder":1,"answer":"blue"}`)

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"用户不存在必须 401（防枚举，且绝不返回 success）")
	assert.NotContains(t, w.Body.String(), `"success":true`)
}

func TestAuthHandler_VerifySecurityAnswer_CorrectAnswer_Returns200(t *testing.T) {
	router := newAuthRouter(t, &securityAnswerUserClient{knownUser: "alice", knownAnswer: "blue"})

	w := postJSON(router, "/api/v1/auth/verify-security-answer",
		`{"username":"alice","questionOrder":1,"answer":"blue"}`)

	require.Equal(t, http.StatusOK, w.Code)
	var data struct {
		Success bool `json:"success"`
	}
	decodeData(t, w.Body.Bytes(), &data)
	assert.True(t, data.Success)
}

func TestAuthHandler_VerifySecurityAnswer_MissingAnswer_Returns400(t *testing.T) {
	router := newAuthRouter(t, &securityAnswerUserClient{knownUser: "alice", knownAnswer: "blue"})

	w := postJSON(router, "/api/v1/auth/verify-security-answer",
		`{"username":"alice","questionOrder":1,"answer":""}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_VerifySecurityAnswer_QuestionOrderOutOfRange_Returns400(t *testing.T) {
	router := newAuthRouter(t, &securityAnswerUserClient{knownUser: "alice", knownAnswer: "blue"})

	for _, order := range []int{0, 3} {
		w := postJSON(router, "/api/v1/auth/verify-security-answer",
			`{"username":"alice","questionOrder":`+strconv.Itoa(order)+`,"answer":"blue"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, "questionOrder=%d 应 400", order)
	}
}
