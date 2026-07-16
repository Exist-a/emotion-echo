package service

import (
	"context"
	"fmt"
	"time"

	"emotion-echo-gin/internal/pkg/memory"
	"emotion-echo-gin/internal/pkg/nanoid"
	"github.com/tmc/langchaingo/llms"
)

// StreamContext 流式处理上下文
type StreamContext struct {
	Req              *StreamRequest
	UserID           int64
	ConvID           string
	UserMsgID        string
	AIMsgID          string
	DetectedEmotion  string
	IsNewConversation bool
}

// StartStream 开始流式处理
func (s *AIService) StartStream(ctx context.Context, streamCtx *StreamContext) <-chan StreamResponse {
	respChan := make(chan StreamResponse, 10)

	go func() {
		defer close(respChan)

		aiMsgID := nanoid.GenerateWithPrefix("msg")
		streamCtx.AIMsgID = aiMsgID

		fmt.Printf("│ AI消息ID: %s\n", aiMsgID)
		fmt.Printf("│ 用户消息ID: %s\n", streamCtx.UserMsgID)
		fmt.Println("╠───────────────────────────────────────────────────────────────────╣")

		respChan <- StreamResponse{
			Event: &StreamEvent{
				Type:           "start",
				ConversationID: streamCtx.ConvID,
				UserMessageID:  streamCtx.UserMsgID,
				MessageID:      aiMsgID,
			},
		}

		fullResponse := s.executeLLMCall(ctx, streamCtx)

		s.streamResponse(ctx, respChan, streamCtx, fullResponse)
	}()

	return respChan
}

// executeLLMCall 执行LLM调用
func (s *AIService) executeLLMCall(ctx context.Context, streamCtx *StreamContext) string {
	var fullResponse string
	var err error

	emotionResult := s.AnalyzeEmotion(ctx, streamCtx.Req)
	streamCtx.DetectedEmotion = emotionResult.Emotion

	s.UpdateMessageEmotion(ctx, streamCtx.UserMsgID, emotionResult.Emotion, emotionResult.Intent)

	if emotionResult.Emotion != "" && s.analysisRepo != nil {
		go s.SaveEmotionAnalysisAsync(ctx, streamCtx.UserID, streamCtx.ConvID,
			emotionResult.Emotion, emotionResult.Confidence, emotionResult.AllScores)
	}

	systemPrompt := emotionResult.SystemPrompt
	if ShouldAddSurveyContext(emotionResult.Emotion) {
		profileContext := s.BuildSurveyContext(ctx, streamCtx.UserID, emotionResult.Emotion)
		if profileContext != "" {
			systemPrompt = systemPrompt + "\n\n" + profileContext
			fmt.Printf("│ 已添加用户档案上下文 (长度: %d)\n", len(profileContext))
		}
	}

	if s.chain == nil {
		fmt.Printf("│ [错误] Chain 未配置，无法处理请求\n")
		return "抱歉，我现在无法回复您，请稍后再试。"
	}

	convMem := s.getOrCreateConversationMemory(streamCtx.ConvID)
	s.reloadConversationMemory(ctx, convMem, streamCtx.ConvID)

	intent := emotionResult.Intent
	if intent == "" {
		intent = "other"
	}

	llmHistory, _ := convMem.LoadMessages(ctx)

	s.logLLMDetails(systemPrompt, intent, emotionResult.Emotion, llmHistory)

	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Println("│                       正在调用 LLM...                              │")

	fullResponse, err = s.chain.CallWithLLMChat(ctx, systemPrompt, llmHistory, intent)
	if err != nil {
		fmt.Printf("│ [错误] LLM 调用失败: %v\n", err)
		fullResponse = "抱歉，我现在无法回复您，请稍后再试。"
	} else {
		fmt.Printf("│ [成功] LLM 返回内容 (长度: %d)\n", len(fullResponse))
	}

	s.saveToMemory(ctx, convMem, streamCtx.Req.Message, fullResponse)

	return fullResponse
}

// getOrCreateConversationMemory 获取或创建会话内存
func (s *AIService) getOrCreateConversationMemory(convID string) *memory.ConversationMemory {
	convMem, exists := s.convMemoryCache[convID]
	if !exists {
		convMem, _ = memory.NewConversationMemory(s.cfg, s.chain.GetLLM())
		s.convMemoryCache[convID] = convMem
	}
	return convMem
}

// reloadConversationMemory 重新加载会话内存
func (s *AIService) reloadConversationMemory(ctx context.Context, convMem *memory.ConversationMemory, convID string) {
	messages, _, _ := s.msgService.List(ctx, 0, convID, 100, 0)
	fmt.Printf("│ [DB] 从数据库加载 %d 条历史消息\n", len(messages))

	_ = convMem.Clear(ctx)
	_ = convMem.LoadMessagesFromModels(ctx, messages)

	testMsgs, _ := convMem.LoadMessages(ctx)
	fmt.Printf("│ [内存] 重新加载后有 %d 条消息\n", len(testMsgs))
}

// logLLMDetails 记录LLM调用详情
func (s *AIService) logLLMDetails(systemPrompt, intent, emotion string, llmHistory []llms.ChatMessage) {
	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Printf("│ 用户消息: %s\n", intent)
	fmt.Printf("│ 意图类型: %s\n", intent)
	fmt.Printf("│ 情绪状态: %s\n", emotion)
	fmt.Printf("│ 历史消息数: %d\n", len(llmHistory))
	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Println("│                     系统提示词 (前200字)                           │")
	sysPreview := systemPrompt
	if len(sysPreview) > 200 {
		sysPreview = sysPreview[:200] + "..."
	}
	fmt.Printf("│ %s\n", sysPreview)
	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Println("│                     上下文历史详情                                  │")
	for i, m := range llmHistory {
		contentPreview := m.GetContent()
		if len(contentPreview) > 80 {
			contentPreview = contentPreview[:80] + "..."
		}
		msgType := "用户"
		if m.GetType() == llms.ChatMessageTypeAI {
			msgType = "AI  "
		}
		fmt.Printf("│ [%2d] [%s] %s\n", i, msgType, contentPreview)
	}
}

// saveToMemory 保存到内存
func (s *AIService) saveToMemory(ctx context.Context, convMem *memory.ConversationMemory, userMsg, aiResponse string) {
	if convMem == nil {
		return
	}
	userPreview := userMsg
	if len(userPreview) > 20 {
		userPreview = userPreview[:20] + "..."
	}
	aiPreview := aiResponse
	if len(aiPreview) > 50 {
		aiPreview = aiPreview[:50] + "..."
	}
	fmt.Printf("│ [保存] 调用前: req.Message='%s', fullResponse='%s...'\n", userPreview, aiPreview)
	_ = convMem.SaveContext(ctx, userMsg, aiResponse)
	fmt.Printf("│ [保存] SaveContext 调用完成\n")

	afterSave, _ := convMem.LoadMessages(ctx)
	fmt.Printf("│ [验证] SaveContext 后历史记录有 %d 条\n", len(afterSave))
}

// streamResponse 流式返回响应
func (s *AIService) streamResponse(ctx context.Context, respChan chan StreamResponse, streamCtx *StreamContext, fullResponse string) {
	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Println("│                       正在流式返回...                               │")

	for _, r := range fullResponse {
		select {
		case <-ctx.Done():
			fmt.Printf("│ [截断] 客户端取消请求，已发送 %d 字符\n", len(fullResponse))
			respChan <- StreamResponse{
				Event: &StreamEvent{
					Type:    "truncated",
					Content: fullResponse,
				},
			}
			if fullResponse != "" {
				_, _ = s.msgService.SaveAIResponse(ctx, streamCtx.ConvID, fullResponse)
				fmt.Printf("│ [截断] 部分回复已保存到数据库\n")
			}
			return
		default:
			respChan <- StreamResponse{
				Event: &StreamEvent{
					Type:    "delta",
					Content: string(r),
				},
			}
			time.Sleep(30 * time.Millisecond)
		}
	}

	fmt.Printf("│ [完成] AI 回复已发送 (总长度: %d)\n", len(fullResponse))
	_, _ = s.msgService.SaveAIResponse(ctx, streamCtx.ConvID, fullResponse)
	fmt.Printf("│ [保存] AI 回复已保存到数据库\n")

	if streamCtx.IsNewConversation || streamCtx.Req.ShouldGenerateTitle {
		s.generateTitle(ctx, respChan, streamCtx)
	}

	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Println("║                    AI 对话请求完成                                  ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")

	respChan <- StreamResponse{
		Event: &StreamEvent{
			Type:           "finish",
			ConversationID: streamCtx.ConvID,
			MessageID:      streamCtx.AIMsgID,
			Emotion:        streamCtx.DetectedEmotion,
		},
	}
}

// generateTitle 生成会话标题
func (s *AIService) generateTitle(ctx context.Context, respChan chan StreamResponse, streamCtx *StreamContext) {
	if s.chain == nil {
		return
	}

	title, err := s.chain.GenerateTitle(ctx, streamCtx.Req.Message)
	if err != nil {
		return
	}

	_ = s.convService.UpdateTitle(ctx, streamCtx.ConvID, title)
	fmt.Printf("│ [标题] 已生成会话标题: %s\n", title)
	respChan <- StreamResponse{
		Event: &StreamEvent{
			Type:           "title_updated",
			ConversationID: streamCtx.ConvID,
			Title:          title,
		},
	}
}
