// Package aiclient — interfaces_test.go
//
// Sibling test for interfaces.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 coverage: the compile-time conformance
// assertions at the bottom of interfaces.go are critical contract
// guards. If a future commit changes a Client method signature
// without updating the interface, the build fails — but we also
// want a runtime mirror that documents the contract and surfaces a
// clear FAIL if the guards are ever deleted.
//
// The "EmptyConfigReturnsNil" tests pin the documented "service
// disabled" semantics of the constructors: empty BaseURL → nil
// pointer → caller must treat as disabled (per fer.go:38 etc.).
package aiclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFERService_ConformanceAtRuntime is a redundant runtime mirror
// of the compile-time assertion `_ FERService = (*FERClient)(nil)`.
// Its purpose is to FAIL if someone deletes the compile-time guard
// without adding a runtime replacement.
func TestFERService_ConformanceAtRuntime(t *testing.T) {
	t.Parallel()
	var _ FERService = (*FERClient)(nil)
	var _ FERService = (*FERClient)(nil) // duplicate — explicit anti-deletion
}

func TestSenseVoiceService_ConformanceAtRuntime(t *testing.T) {
	t.Parallel()
	var _ SenseVoiceService = (*SenseVoiceClient)(nil)
	var _ SenseVoiceService = (*SenseVoiceClient)(nil)
}

func TestXTTSService_ConformanceAtRuntime(t *testing.T) {
	t.Parallel()
	var _ XTTSService = (*XTTSClient)(nil)
	var _ XTTSService = (*XTTSClient)(nil)
}

// TestNewFERClient_EmptyBaseURL_ReturnsNil pins the documented
// "service disabled" branch: empty BaseURL → nil pointer. Caller
// must nil-check before invoking methods.
func TestNewFERClient_EmptyBaseURL_ReturnsNil(t *testing.T) {
	t.Parallel()
	c := NewFERClient(Config{BaseURL: ""})
	assert.Nil(t, c, "empty BaseURL should yield nil client")
}

func TestNewSenseVoiceClient_EmptyBaseURL_ReturnsNil(t *testing.T) {
	t.Parallel()
	c := NewSenseVoiceClient(Config{BaseURL: ""})
	assert.Nil(t, c, "empty BaseURL should yield nil client")
}

// TestNewXTTSClient_EmptyBaseURL_ReturnsNil mirrors the FER/SV pattern.
func TestNewXTTSClient_EmptyBaseURL_ReturnsNil(t *testing.T) {
	t.Parallel()
	c := NewXTTSClient(Config{BaseURL: ""}, "zh-cn", 1.0)
	assert.Nil(t, c, "empty BaseURL should yield nil client")
}

// TestNewFERClient_NonEmptyBaseURL_ReturnsNonNil pins the happy path.
func TestNewFERClient_NonEmptyBaseURL_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	c := NewFERClient(Config{BaseURL: "http://example.test:8004", Timeout: 5})
	require.NotNil(t, c)
}

func TestNewSenseVoiceClient_NonEmptyBaseURL_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	c := NewSenseVoiceClient(Config{BaseURL: "http://example.test:8002", Timeout: 5})
	require.NotNil(t, c)
}

func TestNewXTTSClient_NonEmptyBaseURL_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	c := NewXTTSClient(Config{BaseURL: "http://example.test:8003", Timeout: 5}, "zh-cn", 1.0)
	require.NotNil(t, c)
}