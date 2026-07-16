package analyzer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-ai-svc/internal/aiclient"
)

type stubAnalyzer struct {
	text  string
	model string
}

func (s *stubAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	if text == "" {
		return &EmotionResult{PrimaryEmotion: "neutral", Model: "stub"}, nil
	}
	return &EmotionResult{
		PrimaryEmotion: "happy",
		Confidence:     0.5,
		SentimentScore: 0.3,
		Model:          s.model,
	}, nil
}

func TestMultiModalAnalyzer_TextFallback(t *testing.T) {
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, nil, nil, nil)
	r, err := m.Analyze(context.Background(), MultiModalInput{Kind: "text", Text: "今天很开心"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if r.PrimaryEmotion != "happy" {
		t.Errorf("text path: %+v", r)
	}
}

func TestMultiModalAnalyzer_NoClients_ImageFallsBack(t *testing.T) {
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, nil, nil, nil)
	r, err := m.Analyze(context.Background(), MultiModalInput{Kind: "image", Bytes: []byte("jpeg")})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if r.Model != "stub" {
		t.Errorf("model: %s", r.Model)
	}
}

func TestMultiModalAnalyzer_FERSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1024)
		_, _, _ = r.FormFile("file")
		_ = json.NewEncoder(w).Encode(aiclient.FERResult{
			Emotion: "happy", Confidence: 0.9, Source: "fer",
		})
	}))
	defer srv.Close()

	fer := aiclient.NewFERClient(aiclient.Config{BaseURL: srv.URL, Timeout: 5})
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, fer, nil, nil)

	r, err := m.Analyze(context.Background(), MultiModalInput{
		Kind: "image", Bytes: []byte("jpeg-bytes"), Filename: "face.jpg",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if r.PrimaryEmotion != "happy" || r.Confidence != 0.9 {
		t.Errorf("FER path: %+v", r)
	}
	if r.Model != "fer:fer" {
		t.Errorf("model: %s", r.Model)
	}
}

func TestMultiModalAnalyzer_FERFailure_Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	fer := aiclient.NewFERClient(aiclient.Config{BaseURL: srv.URL, Timeout: 5})
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, fer, nil, nil)

	r, err := m.Analyze(context.Background(), MultiModalInput{
		Kind: "image", Bytes: []byte("jpeg"), Filename: "face.jpg",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if r.Model != "stub" {
		t.Errorf("model after FER failure: %s", r.Model)
	}
}

func TestMultiModalAnalyzer_SenseVoiceSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1024)
		_, _, _ = r.FormFile("file")
		_ = json.NewEncoder(w).Encode(aiclient.SenseVoiceResult{
			Text: "我太开心了", Emotion: "happy", Confidence: 0.95,
			Source: "sensevoice",
		})
	}))
	defer srv.Close()

	sv := aiclient.NewSenseVoiceClient(aiclient.Config{BaseURL: srv.URL, Timeout: 5})
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, nil, sv, nil)

	r, err := m.Analyze(context.Background(), MultiModalInput{
		Kind: "audio", Bytes: []byte("audio-webm-bytes"), Filename: "v.webm",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if r.PrimaryEmotion != "happy" || r.Confidence != 0.95 {
		t.Errorf("audio path: %+v", r)
	}
}

func TestMultiModalAnalyzer_SynthesizeText_NoXTTS(t *testing.T) {
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, nil, nil, nil)
	_, _, err := m.SynthesizeText(context.Background(), "hello")
	if !errors.Is(err, aiclient.ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestMultiModalAnalyzer_SynthesizeText_WithXTTS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(aiclient.TTSResponse{
			Audio: base64.StdEncoding.EncodeToString([]byte("WAV1")),
			SampleRate: 24000,
		})
	}))
	defer srv.Close()

	xtts := aiclient.NewXTTSClient(aiclient.Config{BaseURL: srv.URL, Timeout: 10}, "zh-cn", 0.75)
	stub := &stubAnalyzer{model: "stub"}
	m := NewMultiModalAnalyzer(stub, nil, nil, xtts)

	wav, sr, err := m.SynthesizeText(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SynthesizeText: %v", err)
	}
	if string(wav) != "WAV1" || sr != 24000 {
		t.Errorf("wav/sr: %v %d", wav, sr)
	}
}

// unused: 防止 import io / multipart 警告
var _ = io.Discard
var _ = multipart.ErrMessageTooLarge
