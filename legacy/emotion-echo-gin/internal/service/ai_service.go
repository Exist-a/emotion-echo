package service

import (
	"context"
	"fmt"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/llm"
	"emotion-echo-gin/internal/pkg/memory"
	"emotion-echo-gin/internal/pkg/nanoid"
	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/graph"
	"emotion-echo-gin/internal/workflow/text"
	"gorm.io/gorm"
)

// AIService AI服务
type AIService struct {
	cfg             *config.Config
	db              *gorm.DB
	convService     *ConversationService
	msgService      *MessageService
	surveyService   *SurveyService
	emotionWorkflow *graph.Graph
	analysisRepo    *repository.EmotionAnalysisRepository
	llmCaller       text.LLMCaller
	chain           *llm.Chain
	convMemoryCache map[string]*memory.ConversationMemory
}

// NewAIService 创建AI服务
func NewAIService(cfg *config.Config, db *gorm.DB, convService *ConversationService, msgService *MessageService, surveyService *SurveyService, emotionWorkflow *graph.Graph, analysisRepo *repository.EmotionAnalysisRepository, llmCaller text.LLMCaller, chain *llm.Chain) *AIService {
	return &AIService{
		cfg:             cfg,
		db:              db,
		convService:     convService,
		msgService:      msgService,
		surveyService:   surveyService,
		emotionWorkflow: emotionWorkflow,
		analysisRepo:    analysisRepo,
		llmCaller:       llmCaller,
		chain:           chain,
		convMemoryCache: make(map[string]*memory.ConversationMemory),
	}
}

// StreamChat 流式对话
func (s *AIService) StreamChat(ctx context.Context, userID int64, req *StreamRequest) (<-chan StreamResponse, error) {
	s.logRequestStart(userID, req)

	convID, isNewConversation, userMsgID, err := s.prepareConversation(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	streamCtx := &StreamContext{
		Req:              req,
		UserID:           userID,
		ConvID:           convID,
		UserMsgID:        userMsgID,
		IsNewConversation: isNewConversation,
	}

	return s.StartStream(ctx, streamCtx), nil
}

// logRequestStart 记录请求开始
func (s *AIService) logRequestStart(userID int64, req *StreamRequest) {
	fmt.Println("")
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    AI 对话请求开始                                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Printf("│ 用户ID: %d\n", userID)
	fmt.Printf("│ 会话ID: %s (请求参数)\n", req.ConversationID)
	fmt.Printf("│ 消息内容: %s\n", req.Message)
	fmt.Printf("│ 情绪标签: %s\n", req.Emotion)
	fmt.Printf("│ 语音情绪: %s\n", req.VoiceEmotion)
	fmt.Printf("│ 生成标题: %v\n", req.ShouldGenerateTitle)
}

// prepareConversation 准备会话
func (s *AIService) prepareConversation(ctx context.Context, userID int64, req *StreamRequest) (string, bool, string, error) {
	convID := req.ConversationID
	isNewConversation := false

	if convID == "" {
		isNewConversation = true
		conv, err := s.convService.Create(ctx, userID, &CreateRequest{
			Title: llm.TruncateString(req.Message, 20),
		})
		if err != nil {
			fmt.Printf("│ [错误] 创建会话失败: %v\n", err)
			fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
			return "", false, "", err
		}
		convID = conv.ID
		fmt.Printf("│ [新建会话] 新会话已创建: %s\n", convID)
	} else {
		_, err := s.convService.GetByID(ctx, userID, convID)
		if err != nil {
			fmt.Printf("│ [错误] 获取会话失败: %v\n", err)
			fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
			return "", false, "", err
		}
		fmt.Printf("│ [已有会话] 使用现有会话: %s\n", convID)
	}
	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")

	var userMsgID string
	if req.VoiceEmotion == "" {
		userMsgID = s.saveUserMessage(ctx, convID, req)
		if userMsgID == "" {
			return "", false, "", fmt.Errorf("failed to save user message")
		}
	}

	return convID, isNewConversation, userMsgID, nil
}

// saveUserMessage 保存用户消息
func (s *AIService) saveUserMessage(ctx context.Context, convID string, req *StreamRequest) string {
	var userMsgID string

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fiveSecondsAgo := time.Now().Add(-5 * time.Second).UnixMilli()
		var existingMsg models.Message
		if err := tx.Where("conversation_id = ? AND sender = ? AND content = ? AND send_time > ?",
			convID, "user", req.Message, fiveSecondsAgo).
			Order("send_time DESC").
			First(&existingMsg).Error; err == nil {
			userMsgID = existingMsg.ID
			fmt.Printf("│ [去重] 检测到5秒内重复消息，复用已有消息ID: %s\n", userMsgID)
			return nil
		}

		var duplicateCount int64
		tx.Model(&models.Message{}).
			Where("conversation_id = ? AND sender = ? AND content = ?", convID, "user", req.Message).
			Count(&duplicateCount)
		if duplicateCount > 0 {
			fmt.Printf("│ [警告] 历史消息中发现 %d 条重复内容\n", duplicateCount)
		}

		msg := &models.Message{
			ID:             nanoid.GenerateWithPrefix("msg"),
			ConversationID: convID,
			Sender:        "user",
			Content:       req.Message,
			ContentType:   "text",
			EmotionTag:    &req.Emotion,
			SendTime:      time.Now().UnixMilli(),
			CreatedAt:     time.Now().Unix(),
		}
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		userMsgID = msg.ID
		fmt.Printf("│ [保存] 用户消息已保存: ID=%s, 内容='%s'\n", userMsgID, req.Message)

		preview := req.Message
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		t := time.Now()
		return tx.Model(&models.Conversation{}).Where("id = ?", convID).Updates(map[string]interface{}{
			"last_message_content": preview,
			"last_message_time":     t.UnixMilli(),
			"updated_at":           t,
		}).Error
	}); err != nil {
		fmt.Printf("│ [错误] 保存消息事务失败: %v\n", err)
		fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
		return ""
	}

	fmt.Println("╠───────────────────────────────────────────────────────────────────╣")
	fmt.Println("│                    情绪与意图分析                                    │")

	return userMsgID
}
