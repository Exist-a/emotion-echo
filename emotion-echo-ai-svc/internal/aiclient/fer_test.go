// Package aiclient — fer_test.go
//
// Sibling test for fer.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.6: split ai_client_test.go (which contained
// tests for all three clients) into per-client sibling files. This
// file is the FER portion. Tests were moved verbatim from
// ai_client_test.go and re-themed under fer_test.go.
package aiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewFERClient_NilOnEmptyBaseURL covers the "service disabled"
// branch: empty BaseURL → constructor returns nil → caller must
// check before invoking methods.
func TestNewFERClient_NilOnEmptyBaseURL(t *testing.T) {
	c := NewFERClient(Config{BaseURL: ""})
	if c != nil {
		t.Errorf("expected nil, got %+v", c)
	}
}

// TestNewFERClient_CreatedWhenBaseURLSet covers the happy constructor
// path: BaseURL + Timeout set → client is non-nil and the timeout
// is preserved (converted from seconds to time.Duration).
func TestNewFERClient_CreatedWhenBaseURLSet(t *testing.T) {
	c := NewFERClient(Config{BaseURL: "http://x:8004", Timeout: 5})
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout: got %v, want 5s", c.timeout)
	}
}

// TestFERClient_AnalyzeImage_Success covers the canonical analyze
// path: a /analyze endpoint returns a valid JSON body with emotion +
// confidence + scores; the client decodes them into FERResult.
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

// TestFERClient_AnalyzeImage_UpstreamError covers the "5xx from
// upstream" branch: the client returns an *ErrUpstream carrying
// the status code so the caller can map it to a 502 / 503.
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

// TestFERClient_AnalyzeImage_EmptyBytes covers the local validation
// branch: nil/empty image bytes → client refuses before the network
// round-trip (a free 400).
func TestFERClient_AnalyzeImage_EmptyBytes(t *testing.T) {
	c := NewFERClient(Config{BaseURL: "http://x", Timeout: 5})
	_, err := c.AnalyzeImage(context.Background(), nil, "x.jpg")
	if err == nil || !strings.Contains(err.Error(), "empty image bytes") {
		t.Errorf("expected empty-bytes error, got %v", err)
	}
}

// TestFERClient_NilPointer_AnalyzeImage_ReturnsErrNotConfigured
// covers the "client not configured" branch: a nil *FERClient must
// safely return ErrNotConfigured when its method is called (e.g.
// via interface). Note: in production the constructor returns nil
// when BaseURL is empty, and callers check nil before invoking.
// This test pins the interface-dispatch safety.
func TestFERClient_NilPointer_AnalyzeImage_ReturnsErrNotConfigured(t *testing.T) {
	var c *FERClient // nil
	_, err := c.AnalyzeImage(context.Background(), []byte("x"), "x")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected not-configured error from nil receiver, got %v", err)
	}
}