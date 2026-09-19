// Package handler — security_questions_handler_test.go
//
// E2E-07 TDD: GetSecurityQuestionsHandler HTTP 测试
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	"github.com/emotion-echo/shared/pkg/password"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUserHandlerSvcCtxWithSecurity(repo repository.UserRepo, secRepo repository.SecurityAnswerRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, UserRepo: repo, SecurityAnswerRepo: secRepo}
}

func TestGetSecurityQuestionsHandler_HasQuestions_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	secRepo := repository.NewInMemorySecurityAnswerRepo()

	// 创建用户
	hash, _ := password.Hash("password")
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:           1,
		Username:     "alice",
		PasswordHash: &hash,
	}))

	// 保存密保问题
	answerHash, _ := password.Hash("answer")
	require.NoError(t, secRepo.Save(context.Background(), []*model.SecurityAnswer{
		{UserID: 1, QuestionOrder: 1, Question: "你的第一只宠物叫什么？", AnswerHash: answerHash},
		{UserID: 1, QuestionOrder: 2, Question: "你的出生城市是哪里？", AnswerHash: answerHash},
	}))

	r := gin.New()
	r.GET("/api/v1/users/security-questions", GetSecurityQuestionsHandler(newUserHandlerSvcCtxWithSecurity(repo, secRepo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/security-questions?username=alice", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "你的第一只宠物叫什么？")
	assert.Contains(t, w.Body.String(), "你的出生城市是哪里？")
	// 不得泄露 answer_hash
	assert.NotContains(t, w.Body.String(), "answer_hash")
	assert.NotContains(t, w.Body.String(), "$2a$")
}

func TestGetSecurityQuestionsHandler_UserNotFound_Returns200_EmptyList(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	secRepo := repository.NewInMemorySecurityAnswerRepo()

	r := gin.New()
	r.GET("/api/v1/users/security-questions", GetSecurityQuestionsHandler(newUserHandlerSvcCtxWithSecurity(repo, secRepo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/security-questions?username=nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 防枚举：用户不存在返回 200 + 空列表
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"questions":[]`)
}

func TestGetSecurityQuestionsHandler_MissingUsername_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	secRepo := repository.NewInMemorySecurityAnswerRepo()

	r := gin.New()
	r.GET("/api/v1/users/security-questions", GetSecurityQuestionsHandler(newUserHandlerSvcCtxWithSecurity(repo, secRepo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/security-questions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "username is required")
}