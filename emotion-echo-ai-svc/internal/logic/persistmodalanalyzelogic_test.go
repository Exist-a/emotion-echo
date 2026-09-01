// Package logic — Stage 34 · PR-7 RED
//
// PersistMultiModalAnalyzeLogic 是 MultiModalAnalyzeLogic 的"持久化包装器"：
// 在原有 Analyze 行为之上，根据 kind 与 persist 开关决定是否落库到
// face_emotion_results / voice_emotion_results。
//
// 设计原则：
//   - 复用 analyzer.MultiModalAnalyzer，不重复分析路径
//   - persist=false 时行为与原 MultiModalAnalyzeLogic 完全一致（不写库）
//   - persist=true 时根据 kind 选择对应 repo 写入
//     - image → FaceEmotionRepo.Create
//     - audio → VoiceEmotionRepo.Create
//     - text → 不写库（文本情绪走 Kafka 异步链路，不在 multimodal 端点写）
//   - upload_id 来自 handler 透传（前端 nonce），message_id 可空
package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubMultiModalForPersist 与 multimodalanalyzelogic_test.go 中的 stub 相同，
// 此处独立声明便于本文件单测不依赖其他 test 文件。
type stubMultiModalForPersist struct {
	result *analyzer.EmotionResult
	err    error
}

func (s *stubMultiModalForPersist) Analyze(ctx context.Context, text string) (*analyzer.EmotionResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &analyzer.EmotionResult{PrimaryEmotion: "neutral", Model: "stub"}, nil
}

var _ analyzer.Analyzer = (*stubMultiModalForPersist)(nil)

// TestPersistMultiModalAnalyzeLogic_NotPersist_NoWrite persist=false 不写库。
func TestPersistMultiModalAnalyzeLogic_NotPersist_NoWrite(t *testing.T) {
	t.Parallel()
	faceRepo := repository.NewInMemoryFaceEmotionRepo()
	voiceRepo := repository.NewInMemoryVoiceEmotionRepo()

	stub := &stubMultiModalForPersist{result: &analyzer.EmotionResult{PrimaryEmotion: "happy", Confidence: 0.9, Model: "stub"}}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{
		MultiModal:           mma,
		FaceEmotionRepo:      faceRepo,
		VoiceEmotionRepo:     voiceRepo,
	}

	l := NewPersistMultiModalAnalyzeLogic(svcCtx)
	// image kind + persist=false：不调用 FaceEmotionRepo
	_, err := l.Analyze(context.Background(), PersistRequest{
		Kind: "image", Bytes: []byte("jpeg-bytes"), Filename: "f.jpg",
		UploadID: "upload-001", Persist: false,
	})
	require.NoError(t, err)
	// faceRepo 不应有写入
	got, _ := faceRepo.GetByUploadID(context.Background(), "upload-001")
	assert.Nil(t, got, "persist=false must not write to face repo")
}

// TestPersistMultiModalAnalyzeLogic_ImageKind_PersistToFace image + persist=true → 写 face repo。
func TestPersistMultiModalAnalyzeLogic_ImageKind_PersistToFace(t *testing.T) {
	t.Parallel()
	faceRepo := repository.NewInMemoryFaceEmotionRepo()
	voiceRepo := repository.NewInMemoryVoiceEmotionRepo()

	// FER 服务：返回固定 emotion
	// 这里我们用 stub analyzer 在 image 路径下走 fallback（FER=nil），所以用 fallback 的结果
	stub := &stubMultiModalForPersist{result: &analyzer.EmotionResult{
		PrimaryEmotion: "happy", Confidence: 0.85, SentimentScore: 0.4, Model: "stub-v1",
	}}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{
		MultiModal:       mma,
		FaceEmotionRepo:  faceRepo,
		VoiceEmotionRepo: voiceRepo,
	}

	l := NewPersistMultiModalAnalyzeLogic(svcCtx)
	resp, err := l.Analyze(context.Background(), PersistRequest{
		Kind: "image", Bytes: []byte("jpeg"), Filename: "f.jpg",
		UploadID: "upload-face-001", Persist: true,
		MessageID:      100,
		UserID:         7,
		ConversationID: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	got, err := faceRepo.GetByUploadID(context.Background(), "upload-face-001")
	require.NoError(t, err)
	require.NotNil(t, got, "persist=true + image must write face row")
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, int64(100), got.MessageID)
	assert.Equal(t, int64(7), got.UserID)
	assert.Equal(t, "stub-v1", got.Model)
}

// TestPersistMultiModalAnalyzeLogic_AudioKind_PersistToVoice audio + persist=true → 写 voice repo。
func TestPersistMultiModalAnalyzeLogic_AudioKind_PersistToVoice(t *testing.T) {
	t.Parallel()
	faceRepo := repository.NewInMemoryFaceEmotionRepo()
	voiceRepo := repository.NewInMemoryVoiceEmotionRepo()

	stub := &stubMultiModalForPersist{result: &analyzer.EmotionResult{
		PrimaryEmotion: "sad", Confidence: 0.92, SentimentScore: -0.5, Model: "stub-v1",
	}}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{
		MultiModal:       mma,
		FaceEmotionRepo:  faceRepo,
		VoiceEmotionRepo: voiceRepo,
	}

	l := NewPersistMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), PersistRequest{
		Kind: "audio", Bytes: []byte("webm"), Filename: "v.webm",
		UploadID: "upload-voice-001", Persist: true,
		MessageID: 200, UserID: 7, ConversationID: 50,
		Transcript: "我今天不开心", DurationMs: 3000, Language: "zh",
	})
	require.NoError(t, err)

	got, err := voiceRepo.GetByUploadID(context.Background(), "upload-voice-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sad", got.PrimaryEmotion)
	assert.Equal(t, int64(200), got.MessageID)
	assert.Equal(t, "我今天不开心", got.Transcript)
	assert.Equal(t, 3000, got.DurationMs)
	assert.Equal(t, "zh", got.Language)
}

// TestPersistMultiModalAnalyzeLogic_TextKind_NotPersisted text + persist=true → 不写库。
//
// 文本情绪走 Kafka chat-events → consumer 异步链路，不在 multimodal 端点写。
// 这是 by design，避免双写。
func TestPersistMultiModalAnalyzeLogic_TextKind_NotPersisted(t *testing.T) {
	t.Parallel()
	faceRepo := repository.NewInMemoryFaceEmotionRepo()
	voiceRepo := repository.NewInMemoryVoiceEmotionRepo()

	stub := &stubMultiModalForPersist{result: &analyzer.EmotionResult{PrimaryEmotion: "happy", Model: "stub"}}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{
		MultiModal:       mma,
		FaceEmotionRepo:  faceRepo,
		VoiceEmotionRepo: voiceRepo,
	}

	l := NewPersistMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), PersistRequest{
		Kind: "text", TextContent: "你好",
		UploadID: "upload-text-001", Persist: true,
	})
	require.NoError(t, err)

	got, _ := faceRepo.GetByUploadID(context.Background(), "upload-text-001")
	assert.Nil(t, got, "text kind must not write face repo")
	got2, _ := voiceRepo.GetByUploadID(context.Background(), "upload-text-001")
	assert.Nil(t, got2, "text kind must not write voice repo")
}

// TestPersistMultiModalAnalyzeLogic_AnalyzerError_Propagates 分析失败时错误透传，
// 且 persist=true 时不写库。
func TestPersistMultiModalAnalyzeLogic_AnalyzerError_Propagates(t *testing.T) {
	t.Parallel()
	faceRepo := repository.NewInMemoryFaceEmotionRepo()

	stub := &stubMultiModalForPersist{err: errors.New("fer down")}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{
		MultiModal:      mma,
		FaceEmotionRepo: faceRepo,
	}

	l := NewPersistMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), PersistRequest{
		Kind: "image", Bytes: []byte("jpeg"), Filename: "f.jpg",
		UploadID: "upload-fail", Persist: true,
	})
	require.Error(t, err)
	got, _ := faceRepo.GetByUploadID(context.Background(), "upload-fail")
	assert.Nil(t, got, "analyzer error must prevent write")
}

// _ = time.Now 防 unused 警告
var _ = time.Now

// _ = model.FaceEmotionResult 确保 model import 被使用（PR-7 暂时不直接用，
// 留给后续 PR 在 handler 层构造时引用）
var _ model.FaceEmotionResult
