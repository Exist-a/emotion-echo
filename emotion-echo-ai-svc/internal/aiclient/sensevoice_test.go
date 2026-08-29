// Package aiclient — sensevoice_test.go
//
// Sibling test for sensevoice.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.6: split ai_client_test.go → per-client
// sibling files. This file is the SenseVoice portion.
package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewSenseVoiceClient_NilOnEmptyBaseURL covers the "service
// disabled" branch.
func TestNewSenseVoiceClient_NilOnEmptyBaseURL(t *testing.T) {
	c := NewSenseVoiceClient(Config{BaseURL: ""})
	if c != nil {
		t.Errorf("expected nil, got %+v", c)
	}
}

// TestSenseVoiceClient_Analyze_Success covers the canonical analyze
// path: a /analyze endpoint returns a JSON body with text + emotion
// + raw_text; the client decodes them into SenseVoiceResult.
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

// TestSenseVoiceClient_NilPointer_Analyze_ReturnsErrNotConfigured
// covers the "nil receiver via interface dispatch" branch.
func TestSenseVoiceClient_NilPointer_Analyze_ReturnsErrNotConfigured(t *testing.T) {
	var c *SenseVoiceClient
	_, err := c.Analyze(context.Background(), []byte("x"), "x")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected not-configured error from nil receiver, got %v", err)
	}
}