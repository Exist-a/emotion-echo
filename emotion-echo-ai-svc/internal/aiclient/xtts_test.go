// Package aiclient — xtts_test.go
//
// Sibling test for xtts.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.6: split ai_client_test.go → per-client
// sibling files. This file is the XTTS portion.
package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewXTTSClient_NilOnEmptyBaseURL covers the "service disabled"
// branch.
func TestNewXTTSClient_NilOnEmptyBaseURL(t *testing.T) {
	c := NewXTTSClient(Config{BaseURL: ""}, "", 0)
	if c != nil {
		t.Errorf("expected nil, got %+v", c)
	}
}

// TestNewXTTSClient_DefaultsApplied covers the constructor's
// default-filling branch: language="" → "zh-cn", speed=0 → 0.75.
// These match config.yaml's XTTS section defaults and the
// fallback chain in SynthesizeSpeechLogic.
func TestNewXTTSClient_DefaultsApplied(t *testing.T) {
	c := NewXTTSClient(Config{BaseURL: "http://x:8003"}, "", 0)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.language != "zh-cn" || c.speed != 0.75 {
		t.Errorf("defaults: lang=%s speed=%v", c.language, c.speed)
	}
}

// TestXTTSClient_Synthesize_Success covers the canonical synthesize
// path: a /tts endpoint returns a JSON body with base64 audio +
// sample rate; the client decodes + base64-decodes them.
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

// TestXTTSClient_Synthesize_EmptyText covers the local validation
// branch: nil/whitespace text → client refuses before the network
// round-trip. Note: the implementation considers "  " (whitespace)
// non-empty (matches the synthesis logic contract — see synthesizespeechlogic_test.go).
func TestXTTSClient_Synthesize_EmptyText(t *testing.T) {
	c := NewXTTSClient(Config{BaseURL: "http://x", Timeout: 10}, "", 0)
	_, _, err := c.Synthesize(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "empty text") {
		t.Errorf("expected empty-text error, got %v", err)
	}
}

// TestXTTSClient_NilPointer_Synthesize_ReturnsErrNotConfigured covers
// the "nil receiver via interface dispatch" branch.
func TestXTTSClient_NilPointer_Synthesize_ReturnsErrNotConfigured(t *testing.T) {
	var c *XTTSClient
	_, _, err := c.Synthesize(context.Background(), "x")
	if !errors_Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured from nil receiver, got %v", err)
	}
}

// errors_Is is a tiny alias to keep the test file dependency-light;
// go's errors package is imported anyway via aiclient's other tests.
func errors_Is(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}