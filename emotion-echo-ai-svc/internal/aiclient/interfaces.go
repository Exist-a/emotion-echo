// Package aiclient — interface definitions for the 3 AI model clients.
//
// Per AGENTS.md §三.3 ("DB/Redis/Kafka 等副作用 → 必须用 mock 接口 +
// 测试替身"), downstream code that calls the AI clients should depend
// on these interfaces, not the concrete struct types. The concrete
// types *FERClient, *SenseVoiceClient, *XTTSClient already satisfy
// these interfaces implicitly — no production code change required
// beyond field-type changes in internal/svc/servicecontext.go.
//
// These interfaces also document the minimum surface each client
// exposes to the rest of ai-svc. Anything outside this surface is
// package-private and not part of the contract.
//
// Naming: interfaces are prefixed with the service role to avoid
// collision with the concrete struct of the same root name (FERClient
// the struct vs. FERService the interface — both refer to the same
// underlying object but distinguish "what it is" from "what it does").
package aiclient

import "context"

// FERService is the contract for the Facial Expression Recognition
// downstream. Stage 22-C containerized the FER service; ai-svc
// calls AnalyzeImage on it for /api/v1/multimodal/analyze?kind=image.
//
// Health is exercised by aihealthlogic to surface a per-service liveness
// signal in the /api/v1/ai/health response.
type FERService interface {
	AnalyzeImage(ctx context.Context, imageBytes []byte, filename string) (*FERResult, error)
	Health(ctx context.Context) error
}

// SenseVoiceService is the contract for the speech-recognition + speech-
// emotion downstream. Used by analyzer.MultiModalAnalyzer for the
// audio kind and by aihealthlogic for the per-service health check.
type SenseVoiceService interface {
	Analyze(ctx context.Context, audioBytes []byte, filename string) (*SenseVoiceResult, error)
	Health(ctx context.Context) error
}

// XTTSService is the contract for the text-to-speech downstream. Used
// by SynthesizeSpeechLogic (and indirectly by analyzer.MultiModalAnalyzer
// through SynthesizeText) and by aihealthlogic for liveness.
type XTTSService interface {
	Synthesize(ctx context.Context, text string) ([]byte, int, error)
	SynthesizeToWAV(ctx context.Context, text string) ([]byte, error)
	Health(ctx context.Context) error
}

// Compile-time guards: the concrete clients satisfy their respective
// interfaces. If a method signature drifts, the build fails here
// rather than at the call site (per AGENTS.md §1.1: explicit interface
// conformance guards).
var (
	_ FERService        = (*FERClient)(nil)
	_ SenseVoiceService = (*SenseVoiceClient)(nil)
	_ XTTSService       = (*XTTSClient)(nil)
)