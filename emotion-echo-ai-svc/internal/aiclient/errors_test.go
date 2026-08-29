// Package aiclient — errors_test.go
//
// Shared tests for package-level errors (ErrNotConfigured, ErrUpstream)
// and the cross-client Health probe.
//
// Stage 26-T backlog §三 3.6: extracted from ai_client_test.go during
// the sibling split.
package aiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestErrNotConfigured pins the sentinel error returned by each
// client when BaseURL is empty (i.e. service disabled). Callers use
// errors.Is(err, aiclient.ErrNotConfigured) to detect this case
// and map it to a 503.
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

// TestClients_Health_Basic covers the cross-client /health probe
// behavior: each client issues a GET to {BaseURL}/health and
// considers 200 OK → healthy. We use a single test server that
// responds OK for all three clients (the routes they hit are the
// same — the only difference is the upstream they actually represent).
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