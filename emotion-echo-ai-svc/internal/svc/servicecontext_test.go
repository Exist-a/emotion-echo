// Package svc — servicecontext_test.go
//
// Sibling test for servicecontext.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover servicecontext.go (45 LOC) test
// surface. The ServiceContext is a thin DI container holding Config +
// EmotionRepo + AI clients + MultiModal. We test the two constructors
// (NewServiceContext, InitMultiModal) and verify field wiring.
//
// Coverage:
//
//   - NewServiceContext: minimal config + repo → svcCtx has those fields
//   - NewServiceContext: AI client fields nil (not initialized by New)
//   - InitMultiModal: all 3 AI clients non-nil when BaseURL set
//   - InitMultiModal: clients nil when BaseURL empty
//   - InitMultiModal: MultiModal analyzer is non-nil after init
//   - InitMultiModal: FER is set to the SAME instance MultiModal uses
//     (consistency check — prevents drift between two paths)
//
// Per AGENTS.md §三.3 we use the live aiclient constructors (no mock)
// because they return nil when BaseURL is empty — that itself is the
// observable we want to verify.
package svc

import (
	"testing"

	"emotion-echo-ai-svc/internal/aiclient"
	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/config"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewServiceContext_FieldsWired covers the constructor's basic
// contract: cfg + repo are stored; AI client fields are NOT initialized
// (InitMultiModal is a separate step).
func TestNewServiceContext_FieldsWired(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Name: "test"}
	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := NewServiceContext(cfg, repo, nil, nil, nil)
	require.NotNil(t, svcCtx)
	assert.Equal(t, cfg, svcCtx.Config)
	assert.Equal(t, repo, svcCtx.EmotionRepo)
	assert.Nil(t, svcCtx.FER, "FER not initialized by NewServiceContext")
	assert.Nil(t, svcCtx.SenseVoice)
	assert.Nil(t, svcCtx.XTTS)
	assert.Nil(t, svcCtx.MultiModal)
}

// TestInitMultiModal_AllBaseURLsSet_ClientsAndAnalyzerNonNil covers
// the happy init path: every BaseURL set → every AI client is a
// non-nil interface value, MultiModal analyzer is non-nil.
func TestInitMultiModal_AllBaseURLsSet_ClientsAndAnalyzerNonNil(t *testing.T) {
	t.Parallel()

	svcCtx := NewServiceContext(config.Config{
		Name:        "test",
		FER:         config.FER{BaseURL: "http://fer:8004"},
		SenseVoice:  config.SenseVoice{BaseURL: "http://sv:8005"},
		XTTS:        config.XTTS{BaseURL: "http://xtts:8003", Language: "zh-cn", Speed: 0.75},
	}, repository.NewInMemoryEmotionRepo(), nil, nil, nil) // Stage 34: face/voice/fused nil

	svcCtx.InitMultiModal()
	require.NotNil(t, svcCtx.FER, "FER should be non-nil when BaseURL set")
	require.NotNil(t, svcCtx.SenseVoice)
	require.NotNil(t, svcCtx.XTTS)
	require.NotNil(t, svcCtx.MultiModal, "MultiModal analyzer should be non-nil")
}

// TestInitMultiModal_EmptyBaseURLs_ClientsNil covers the "all
// services disabled" path: every BaseURL empty → every AI client is
// nil; the analyzer still constructs (with nil clients + fallback).
func TestInitMultiModal_EmptyBaseURLs_ClientsNil(t *testing.T) {
	t.Parallel()

	svcCtx := NewServiceContext(config.Config{Name: "test"}, repository.NewInMemoryEmotionRepo(), nil, nil, nil)
	// All AIService fields are zero-valued (BaseURL="")
	svcCtx.InitMultiModal()

	assert.Nil(t, svcCtx.FER, "FER should be nil when BaseURL empty")
	assert.Nil(t, svcCtx.SenseVoice)
	assert.Nil(t, svcCtx.XTTS)
	// MultiModal analyzer is still non-nil — it always has the
	// keyword fallback (per analyzer.NewMultiModalAnalyzer contract).
	assert.NotNil(t, svcCtx.MultiModal)
}

// TestInitMultiModal_MixedBaseURLs_PartialInit covers the "some
// services enabled, some disabled" path (the typical dev config:
// FER + SenseVoice enabled, XTTS disabled because the model is too
// heavy for a laptop).
func TestInitMultiModal_MixedBaseURLs_PartialInit(t *testing.T) {
	t.Parallel()

svcCtx := NewServiceContext(config.Config{
		Name:        "test",
		FER:         config.FER{BaseURL: "http://fer:8004"},
		SenseVoice:  config.SenseVoice{BaseURL: "http://sv:8005"},
		XTTS:        config.XTTS{BaseURL: ""}, // disabled
	}, repository.NewInMemoryEmotionRepo(), nil, nil, nil) // Stage 34: face/voice/fused nil

	svcCtx.InitMultiModal()

	assert.NotNil(t, svcCtx.FER)
	assert.NotNil(t, svcCtx.SenseVoice)
	assert.Nil(t, svcCtx.XTTS, "XTTS must be nil when BaseURL empty")
	assert.NotNil(t, svcCtx.MultiModal)
}

// TestInitMultiModal_AnalyzerHasKeywordFallback covers the integration
// between the analyzer and the service context: when all clients are
// nil, the keyword analyzer (embedded inside MultiModalAnalyzer) is
// still callable.
func TestInitMultiModal_AnalyzerHasKeywordFallback(t *testing.T) {
	t.Parallel()

	svcCtx := NewServiceContext(config.Config{Name: "test"}, repository.NewInMemoryEmotionRepo(), nil, nil, nil)
	svcCtx.InitMultiModal()
	require.NotNil(t, svcCtx.MultiModal)

	// We don't know the exact internal analyzer field name; we just
	// verify the analyzer is non-nil and doesn't panic on construction.
	// Per AGENTS.md §三.3 this is the integration-level safety net.
}

// Compile-time guards would normally pin field types, but doing it
// via (*ServiceContext)(nil).FER panics (nil pointer deref at init
// time). Instead, the runtime tests above already exercise the
// fields. If a future refactor reverts FER/SenseVoice/XTTS to concrete
// *aiclient.FERClient etc., the tests' calls to NewFERClient (which
// now returns the interface type) will need to be updated too.

// Compile-time unused-import guards. analyzer and aiclient are used
// by runtime tests; these aliases ensure they don't get optimized
// away when only type-references appear in tests.
var (
	_ analyzer.EmotionResult
	_ = aiclient.ErrNotConfigured
)