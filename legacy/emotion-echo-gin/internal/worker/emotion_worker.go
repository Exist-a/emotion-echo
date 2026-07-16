package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/text"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// EmotionWorker 情绪分析工作器
type EmotionWorker struct {
	cfg          *config.Config
	convRepo     *repository.ConversationRepository
	msgRepo      *repository.MessageRepository
	analysisRepo *repository.EmotionAnalysisRepository
	userRepo     *repository.UserRepository
}

// NewEmotionWorker 创建工作器
func NewEmotionWorker(
	cfg *config.Config,
	convRepo *repository.ConversationRepository,
	msgRepo *repository.MessageRepository,
	analysisRepo *repository.EmotionAnalysisRepository,
	userRepo *repository.UserRepository,
) *EmotionWorker {
	return &EmotionWorker{
		cfg:          cfg,
		convRepo:     convRepo,
		msgRepo:      msgRepo,
		analysisRepo: analysisRepo,
		userRepo:     userRepo,
	}
}

// AnalyzeConversation 分析单个会话
func (w *EmotionWorker) AnalyzeConversation(ctx context.Context, userID int64, conversationID string) error {
	log.Printf("Analyzing conversation %s for user %d", conversationID, userID)

	messages, err := w.msgRepo.ListByConversationID(ctx, conversationID, 100, 0)
	if err != nil {
		return err
	}

	if len(messages) == 0 {
		log.Printf("No messages in conversation %s, skipping", conversationID)
		return nil
	}

	state := text.NewTextState()
	state.SetMessages(messages)

	llmCaller := createLLMCaller(w.cfg)
	state, err = text.RunOfflineWorkflow(ctx, llmCaller, state)
	if err != nil {
		log.Printf("Workflow failed for conversation %s: %v", conversationID, err)
		return err
	}

	emotion := state.GetEmotion()
	confidence := state.GetConfidence()
	emotionScores := buildEmotionScores(emotion, confidence)

	analysis := &models.EmotionAnalysis{
		ID:              0,
		ConversationID:  conversationID,
		UserID:          userID,
		AnalyzedAt:      time.Now(),
		EmotionScores:   emotionScores,
		DominantEmotion: emotion,
		Summary:         state.GetSummary(),
		CreatedAt:       time.Now(),
	}

	if err := w.analysisRepo.Create(ctx, analysis); err != nil {
		log.Printf("Failed to save analysis for conversation %s: %v", conversationID, err)
		return err
	}

	log.Printf("Analysis completed for conversation %s: dominant=%s", conversationID, emotion)
	return nil
}

func createLLMCaller(cfg *config.Config) func(ctx context.Context, prompt string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		var llm llms.Model
		var err error

		if cfg.AI.Provider == "local" {
			llm, err = openai.New(
				openai.WithBaseURL(cfg.AI.Local.BaseURL),
				openai.WithToken("dummy"),
			)
		} else if cfg.AI.Provider == "kimi" {
			llm, err = openai.New(
				openai.WithToken(cfg.AI.Kimi.APIKey),
				openai.WithBaseURL(cfg.AI.Kimi.BaseURL),
			)
		} else {
			llm, err = openai.New(
				openai.WithToken(cfg.AI.Kimi.APIKey),
				openai.WithBaseURL(cfg.AI.Kimi.BaseURL),
			)
		}

		if err != nil {
			return "", err
		}

		completion, err := llm.Call(ctx, prompt,
			llms.WithTemperature(0.3),
			llms.WithMaxTokens(500),
		)
		if err != nil {
			return "", err
		}

		return completion, nil
	}
}

func buildEmotionScores(emotion string, confidence float64) models.EmotionScoresMap {
	scores := make(map[string]float64)
	emotions := []string{"happy", "sad", "angry", "anxious", "fear", "neutral", "calm", "unk", "unknown"}
	for _, e := range emotions {
		if e == emotion {
			scores[e] = confidence
		} else {
			scores[e] = 0.0
		}
	}
	return scores
}

// BatchAnalyze 批量分析（定时任务调用）
func (w *EmotionWorker) BatchAnalyze(ctx context.Context) error {
	log.Println("Starting batch emotion analysis...")

	// 获取昨天的会话（按自然日）
	now := time.Now()
	startTime := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	endTime := startTime.Add(24 * time.Hour)

	// 获取所有活跃用户（简化版：获取所有有消息的用户）
	conversations, err := w.convRepo.ListRecentConversations(ctx, startTime, endTime, 1000)
	if err != nil {
		return err
	}

	log.Printf("Found %d conversations to analyze", len(conversations))

	// 并发分析（限制并发数）
	const maxConcurrency = 5
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failedCount int

	for _, conv := range conversations {
		// 检查是否已分析（按自然日判断：如果已有该日期的分析，跳过）
		existing, err := w.analysisRepo.GetLatestByConversationID(ctx, conv.ID)
		if err != nil {
			log.Printf("Error checking existing analysis: %v", err)
			continue
		}
		
		// 如果今天已经分析过，跳过（自然日判断）
		if existing != nil && isSameDay(existing.AnalyzedAt, now) {
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}
		
		go func(userID int64, convID string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if err := w.AnalyzeConversation(ctx, userID, convID); err != nil {
				log.Printf("Failed to analyze conversation %s: %v", convID, err)
				mu.Lock()
				failedCount++
				mu.Unlock()
			}
		}(conv.UserID, conv.ID)
	}

	wg.Wait()
	log.Printf("Batch emotion analysis completed, failed: %d", failedCount)
	return nil
}

// AnalyzeByMessageThreshold 按消息阈值触发分析
func (w *EmotionWorker) AnalyzeByMessageThreshold(ctx context.Context, userID int64, conversationID string, messageCount int) error {
	threshold := w.cfg.Analysis.ThresholdMessages
	if threshold == 0 {
		threshold = 20 // 默认20条
	}

	if messageCount < threshold {
		return nil
	}

	// 检查是否已分析
	existing, err := w.analysisRepo.GetLatestByConversationID(ctx, conversationID)
	if err != nil {
		return err
	}

	// 如果今天已经分析过，跳过
	if existing != nil && isSameDay(existing.AnalyzedAt, time.Now()) {
		return nil
	}

	return w.AnalyzeConversation(ctx, userID, conversationID)
}

// isSameDay 判断两个时间是否为同一天
func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
