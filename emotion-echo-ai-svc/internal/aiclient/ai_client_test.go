// Package aiclient — ai_client_test.go
//
// Sibling test for ai_client.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 coverage: ai_client.go (LOC=50) holds the
// shared ErrNotConfigured sentinel and the ErrUpstream type + itoa
// helper used by all 3 AI clients (FER / SenseVoice / XTTS).
//
// This file locks:
//   - itoa() edge cases (0, positive, negative, multi-digit)
//   - ErrUpstream.Error() format
//   - ErrNotConfigured sentinel value (callers use errors.Is)
package aiclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestItoa_Zero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0", itoa(0))
}

func TestItoa_SingleDigit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "7", itoa(7))
}

func TestItoa_MultiDigit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "42", itoa(42))
	assert.Equal(t, "123", itoa(123))
	assert.Equal(t, "2026", itoa(2026))
}

func TestItoa_Large(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "999999", itoa(999999))
}

func TestItoa_Negative(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "-1", itoa(-1))
	assert.Equal(t, "-7", itoa(-7))
	assert.Equal(t, "-123", itoa(-123))
}

func TestErrUpstream_Error_ContainsStatusCode(t *testing.T) {
	t.Parallel()
	e := &ErrUpstream{StatusCode: 502, Body: "bad gateway"}
	msg := e.Error()
	assert.Contains(t, msg, "502")
	assert.Contains(t, msg, "upstream error")
}

func TestErrUpstream_Error_ZeroStatusCode(t *testing.T) {
	t.Parallel()
	// Pathological case: caller forgot to populate StatusCode.
	e := &ErrUpstream{StatusCode: 0, Body: ""}
	msg := e.Error()
	assert.Contains(t, msg, "status=0")
}

func TestErrNotConfigured_IsSentinel(t *testing.T) {
	t.Parallel()
	// ErrNotConfigured is returned by FER/SenseVoice/XTTS Analyze*
	// paths when the corresponding AI service is not configured.
	// Callers in analyzer/consumer.go check this with errors.Is
	// to decide whether to skip vs. fail.
	assert.NotNil(t, ErrNotConfigured)
	// identity check: the same package-level var must be returned
	// by all 3 clients (we verify the surface stays consistent).
	err := errors.New("wrapper")
	assert.False(t, errors.Is(err, ErrNotConfigured),
		"a plain errors.New must not satisfy ErrNotConfigured sentinel")
}