package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emotion-echo-chat-svc/internal/logic"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// statusForLogicErr 把 logic 层错误映射成 HTTP 状态码。
//
// 与 gRPC 路径**对齐**：`shared/pkg/grpcerr` 的字符串前缀约定里 `validation:` ⇒
// codes.InvalidArgument ⇒ BFF 400。而 HTTP 降级路径（CHAT_TRANSPORT=http）此前
// 一律回 500 ⇒ **同一个错误在两条路径上状态码不同**，回滚开关一拨归因就变了。
// 这条路径平时无人跑，正是这类漂移最容易潜伏的地方（CL-05 Phase 5 实测：
// 非法游标在 gRPC 路径 400、在 HTTP 路径 500）。
func statusForLogicErr(err error) int {
	if strings.HasPrefix(err.Error(), "validation:") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// CreateConversationHandler POST /api/v1/conversations
func CreateConversationHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.CreateConversationReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewCreateConversationLogic(c.Request.Context(), svcCtx).CreateConversation(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// SendMessageHandler POST /api/v1/conversations/:id/messages
func SendMessageHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.SendMessageReq
		// :id 取自 path
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		req.Id = id
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewSendMessageLogic(c.Request.Context(), svcCtx).SendMessage(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListConversationsHandler GET /api/v1/conversations
//
// Stage 36-A2.1：补齐 chat-svc 缺漏的会话列表端点（G2 上半）。
// 下游 BFF 不再需要返回空 stub。
func ListConversationsHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &types.ListConversationsReq{}
		if limitStr := c.Query("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				req.Limit = n
			}
		}
		if offsetStr := c.Query("offset"); offsetStr != "" {
			if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
				req.Offset = n
			}
		}
		// CL-05 Phase 5：HTTP 降级路径（`CHAT_TRANSPORT=http` 回滚开关）也必须认 `cursor`，
		// 否则翻页在此路径上**静默失效**（与 Phase 6 给 /messages 补 cursor 同一理由）。
		req.Cursor = c.Query("cursor")
		resp, err := logic.NewListConversationsLogic(c.Request.Context(), svcCtx).ListConversations(req)
		if err != nil {
			// CL-05 Phase 5：非法游标属于**客户端非法输入** ⇒ 400（不是 500）。
			// 与 gRPC 路径靠 grpcerr 前缀表得到的状态码保持一致。
			c.JSON(statusForLogicErr(err), gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListMessagesHandler GET /api/v1/conversations/:id/messages
func ListMessagesHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ListMessagesReq
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		req.Id = id
		if limitStr := c.Query("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				req.Limit = n
			}
		}
		// CL-05 Phase 6 · T-7：HTTP 降级路径（`CHAT_TRANSPORT=http` 回滚开关）也必须认
		// `cursor`，否则翻页在此路径上**静默失效**（永远返回最新一页，用户上滚看不到历史）。
		// 语义与 gRPC 主路径 / BFF 的 buildMessageQuery 一致：只在 > 0 时生效，0 = 从最新开始。
		if cursorStr := c.Query("cursor"); cursorStr != "" {
			if n, err := strconv.ParseInt(cursorStr, 10, 64); err == nil && n > 0 {
				req.Cursor = n
			}
		}
		resp, err := logic.NewListMessagesLogic(c.Request.Context(), svcCtx).ListMessages(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// DeleteConversationHandler DELETE /api/v1/conversations/:id
func DeleteConversationHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		resp, err := logic.NewDeleteConversationLogic(c.Request.Context(), svcCtx).
			DeleteConversation(&types.DeleteConversationReq{Id: id})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, repository.ErrNotFound) {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "unauthorized") {
				status = http.StatusForbidden
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// HealthHandler GET /health
func HealthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewHealthLogic(c.Request.Context(), svcCtx).Health()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// HealthReadyHandler 就绪检查（无鉴权，D-29 新增）
//
// readiness 语义：依赖不可用时返 **503**，供 compose healthcheck 判定。
// 与 /health 共用同一套依赖检查（同一个 HealthLogic），差别仅在 HTTP 码 ——
// 两者的响应体 status 字段必然一致。
func HealthReadyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewHealthLogic(c.Request.Context(), svcCtx).Health()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if resp.Status != logic.StatusOk {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, resp)
	}
}