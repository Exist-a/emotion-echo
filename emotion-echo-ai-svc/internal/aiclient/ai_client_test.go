package aiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- New* constructor tests ----------
func TestNewFERClient_NilOnEmptyBaseURL(t *testing.T) {
	c := NewFERClient(Config{BaseURL: ""})
	if c != nil {
		t.Errorf("expected nil, got %+v", c)
	}
}

func TestNewSenseVoiceClient_NilOnEmptyBaseURL(t *testing.T) {
	c := NewSenseVoiceClient(Config{BaseURL: ""})
	if c != nil {
		t.Errorf("expected nil, got %+v", c)
	}
}

func TestNewXTTSClient_NilOnEmptyBaseURL(t *testing.T) {
	c := NewXTTSClient(Config{BaseURL: ""}, "", 0)
	if c != nil {
		t.Errorf("expected nil, got %+v", c)
	}
}

func TestNewFERClient_CreatedWhenBaseURLSet(t *testing.T) {
	c := NewFERClient(Config{BaseURL: "http://x:8004", Timeout: 5})
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout: got %v, want 5s", c.timeout)
	}
}

func TestNewXTTSClient_DefaultsApplied(t *testing.T) {
	c := NewXTTSClient(Config{BaseURL: "http://x:8003"}, "", 0)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.language != "zh-cn" || c.speed != 0.75 {
		t.Errorf("defaults: lang=%s speed=%v", c.language, c.speed)
	}
}

// ---------- FER AnalyzeImage ----------
func TestFERClient_AnalyzeImage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// multipart form should contain file= with filename
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart, got %s", r.Header.Get("Content-Type"))
		}
		_ = r.ParseMultipartForm(1024)
		f, fh, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("no file in form: %v", err)
		}
		defer f.Close()
		if fh.Filename != "test.jpg" {
			t.Errorf("filename: got %s", fh.Filename)
		}
		body, _ := io.ReadAll(f)
		if len(body) == 0 {
			t.Error("empty body")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(FERResult{
			Emotion: "happy", Confidence: 0.93, Source: "fer",
			Scores: map[string]float64{"happy": 0.93, "neutral": 0.07},
		})
	}))
	defer srv.Close()

	c := NewFERClient(Config{BaseURL: srv.URL, Timeout: 5})
	res, err := c.AnalyzeImage(context.Background(), []byte("fake-jpeg-bytes"), "test.jpg")
	if err != nil {
		t.Fatalf("AnalyzeImage: %v", err)
	}
	if res.Emotion != "happy" || res.Confidence != 0.93 || res.Source != "fer" {
		t.Errorf("unexpected result: %+v", res)
	}
	if res.Scores["happy"] != 0.93 {
		t.Errorf("scores: %+v", res.Scores)
	}
}

func TestFERClient_AnalyzeImage_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewFERClient(Config{BaseURL: srv.URL, Timeout: 5})
	_, err := c.AnalyzeImage(context.Background(), []byte("img"), "x.jpg")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*ErrUpstream); !ok {
		t.Errorf("expected ErrUpstream, got %T", err)
	}
}

func TestFERClient_AnalyzeImage_EmptyBytes(t *testing.T) {
	c := NewFERClient(Config{BaseURL: "http://x", Timeout: 5})
	_, err := c.AnalyzeImage(context.Background(), nil, "x.jpg")
	if err == nil || !strings.Contains(err.Error(), "empty image bytes") {
		t.Errorf("expected empty-bytes error, got %v", err)
	}
}

// ---------- SenseVoice Analyze ----------
func TestSenseVoiceClient_Analyze_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1024)
		_, _, _ = r.FormFile("file")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SenseVoiceResult{
			Text: "你好世界", Emotion: "happy", Confidence: 0.9,
			RawText: "<|HAPPY|><|zh|>你好世界", Source: "sensevoice",
		})
	}))
	defer srv.Close()

	c := NewSenseVoiceClient(Config{BaseURL: srv.URL, Timeout: 5})
	res, err := c.Analyze(context.Background(), []byte("audio-webm-bytes"), "voice.webm")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Text != "你好世界" || res.Emotion != "happy" {
		t.Errorf("unexpected: %+v", res)
	}
}

// ---------- XTTS Synthesize ----------
func TestXTTSClient_Synthesize_Success(t *testing.T) {
	// base64-encoded "WAV"
	audio := "V0FWMQ=="                                 // ASCII "WAV1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tts" {
			t.Errorf("path: %s", r.URL.Path)
		}
		var got TTSRequest
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got.Text != "hello" || got.Language != "zh-cn" {
			t.Errorf("payload: %+v", got)
		}
		_ = json.NewEncoder(w).Encode(TTSResponse{
			Audio: audio, SampleRate: 24000, Text: "hello", Language: "zh-cn",
		})
	}))
	defer srv.Close()

	c := NewXTTSClient(Config{BaseURL: srv.URL, Timeout: 10}, "zh-cn", 0.75)
	wav, sr, err := c.Synthesize(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if sr != 24000 {
		t.Errorf("sample rate: %d", sr)
	}
	if string(wav) != "WAV1" {
		t.Errorf("decoded audio mismatch: got %q", string(wav))
	}
}

func TestXTTSClient_Synthesize_EmptyText(t *testing.T) {
	c := NewXTTSClient(Config{BaseURL: "http://x", Timeout: 10}, "", 0)
	_, _, err := c.Synthesize(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "empty text") {
		t.Errorf("expected empty-text error, got %v", err)
	}
}

// ---------- Health probes ----------
func TestClients_Health_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	if err := NewFERClient(Config{BaseURL: srv.URL}).Health(ctx); err != nil {
		t.Errorf("FER health: %v", err)
	}
	if err := NewSenseVoiceClient(Config{BaseURL: srv.URL}).Health(ctx); err != nil {
		t.Errorf("SenseVoice health: %v", err)
	}
	if err := NewXTTSClient(Config{BaseURL: srv.URL}, "", 0).Health(ctx); err != nil {
		t.Errorf("XTTS health: %v", err)
	}
}

// ---------- ErrNotConfigured ----------
func TestErrNotConfigured(t *testing.T) {
	if _, err := NewFERClient(Config{BaseURL: ""}).AnalyzeImage(context.Background(), []byte("x"), "x"); err == nil || !errors.Is(err, ErrNotConfigured) {
		t.Errorf("FER: %v", err)
	}
	if _, err := NewSenseVoiceClient(Config{BaseURL: ""}).Analyze(context.Background(), []byte("x"), "x"); err == nil || !errors.Is(err, ErrNotConfigured) {
		t.Errorf("SenseVoice: %v", err)
	}
	if _, _, err := NewXTTSClient(Config{BaseURL: ""}, "", 0).Synthesize(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("XTTS: %v", err)
	}
}
