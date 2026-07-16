package service

import (
	"context"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/llm"
	"emotion-echo-gin/internal/repository"
)

// VoiceService 语音服务
type VoiceService struct {
	cfg               *config.Config
	asrClient         *llm.ASRClient
	emotionClient     *llm.EmotionClient
	messageRepo       *repository.MessageRepository
	conversationRepo  *repository.ConversationRepository
}

// NewVoiceService 创建语音服务
func NewVoiceService(
	cfg *config.Config,
	messageRepo *repository.MessageRepository,
	conversationRepo *repository.ConversationRepository,
) *VoiceService {
	return &VoiceService{
		cfg:               cfg,
		asrClient:         llm.NewASRClient(cfg),
		emotionClient:     llm.NewEmotionClient(cfg),
		messageRepo:       messageRepo,
		conversationRepo: conversationRepo,
	}
}

// ProcessVoiceMessage 处理语音消息
func (s *VoiceService) ProcessVoiceMessage(
	ctx context.Context,
	userID int64,
	conversationID string,
	audioFile *multipart.FileHeader,
	onProgress func(transcript string),
) (*VoiceProcessResult, error) {
	tmpDir := filepath.Join(os.TempDir(), "emotion-echo", "voice")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	tmpFilePath := filepath.Join(tmpDir, fmt.Sprintf("%d_%d.webm", userID, time.Now().UnixNano()))

	if err := SaveAudioFile(audioFile, tmpFilePath); err != nil {
		return nil, fmt.Errorf("failed to save audio file: %w", err)
	}
	defer os.Remove(tmpFilePath)

	// 只用 SenseVoice 做一次调用：同时拿到文字和情绪
	fmt.Println("====== Processing Voice Message ======")
	fmt.Println("[1] Calling SenseVoice for recognition...")
	emotionResult, err := s.emotionClient.AnalyzeEmotion(ctx, tmpFilePath)
	if err != nil {
		return nil, fmt.Errorf("voice processing failed: %w", err)
	}

	fmt.Println("[2] Recognition results:")
	fmt.Printf("  - Transcript: \"%s\"\n", emotionResult.Text)
	fmt.Printf("  - Detected Emotion: \"%s\"\n", emotionResult.Emotion)
	fmt.Printf("  - Confidence: %.2f\n", emotionResult.Confidence)

	transcript := emotionResult.Text
	if transcript == "" {
		transcript = ""
	}

	if transcript != "" && onProgress != nil {
		onProgress(transcript)
	}

	emotion := "neutral"
	emotionLabel := "中性"
	if emotionResult.Emotion != "" {
		emotion = emotionResult.Emotion
		emotionLabel = llm.GetEmotionLabel(emotion)
	}

	fmt.Println("[3] Final processing results:")
	fmt.Printf("  - Transcript: \"%s\"\n", transcript)
	fmt.Printf("  - Emotion: \"%s\"\n", emotion)
	fmt.Printf("  - Emotion Label: \"%s\"\n", emotionLabel)
	fmt.Println("=====================================")

	saveDir := filepath.Join(s.cfg.Storage.Local.Path, "voice")
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create save dir: %w", err)
	}

	messageID := GenerateMessageID()
	ext := filepath.Ext(audioFile.Filename)
	if ext == "" {
		ext = ".webm"
	}
	audioFileName := fmt.Sprintf("%s%s", messageID, ext)
	savedFilePath := filepath.Join(saveDir, audioFileName)

	if err := SaveAudioFile(audioFile, savedFilePath); err != nil {
		return nil, fmt.Errorf("failed to save audio file to storage: %w", err)
	}

	audioURL := "/uploads/voice/" + audioFileName

	duration := 0
	if emotionResult != nil {
		duration = int(emotionResult.Confidence * 100)
	}

	var emotionTag *string
	if emotion != "" {
		emotionTag = &emotion
	}

	msg := &models.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Sender:         "user",
		Content:        transcript,
		ContentType:    "audio",
		AudioURL:       audioURL,
		AudioDuration:  duration,
		EmotionTag:     emotionTag,
		IntentType:     "emotional",
		SendTime:       time.Now().UnixMilli(),
	}

	if err := s.messageRepo.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	return &VoiceProcessResult{
		MessageID:    messageID,
		Transcript:   transcript,
		Emotion:      emotion,
		EmotionLabel: emotionLabel,
		AudioURL:     audioURL,
		Duration:     duration,
	}, nil
}
