// Package grpcinterceptor · go2sky adapter for ServerTracing
//
// Stage 13 production adapter: bridges go2sky.Tracer to our minimal Tracer interface.
//
// Why split into two files?
//   - tracing.go: pure interface + interceptor, no third-party deps, easy to test
//   - tracing_go2sky.go: depends on github.com/SkyAPM/go2sky
//   - This package remains "light" (no go2sky in test scope) when not needed
//
// Note: go2sky v1.5.0 only exposes CreateLocalSpan / CreateExitSpan, no
// dedicated EntrySpan. For gRPC server tracing, CreateLocalSpan is the
// recommended pattern. See:
// https://github.com/SkyAPM/go2sky/issues/118

package grpcinterceptor

import (
	"context"
	"time"

	"github.com/SkyAPM/go2sky"
)

// Go2SkySpan wraps go2sky's span so it satisfies our Span interface.
type Go2SkySpan struct {
	span go2sky.Span
}

// EndSpan finishes the go2sky span. err indicates success/failure.
func (s *Go2SkySpan) EndSpan(err error) {
	if s.span == nil {
		return
	}
	if err != nil {
		s.span.Error(time.Now(), err.Error())
	}
	s.span.End()
}

// Go2SkyTracer adapts go2sky.Tracer to our minimal Tracer interface.
type Go2SkyTracer struct {
	tracer *go2sky.Tracer
}

// NewGo2SkyTracer wraps an existing go2sky tracer.
func NewGo2SkyTracer(tracer *go2sky.Tracer) *Go2SkyTracer {
	if tracer == nil {
		return nil
	}
	return &Go2SkyTracer{tracer: tracer}
}

// StartEntry implements Tracer.
//
// Uses CreateExitSpanWithContext as a workaround for gRPC server tracing.
// go2sky v1.5 doesn't expose a dedicated EntrySpan for incoming requests.
// The "peer" param is set to the local service name as a placeholder.
func (t *Go2SkyTracer) StartEntry(ctx context.Context, operationName string) (context.Context, Span) {
	if t == nil || t.tracer == nil {
		return ctx, &Go2SkySpan{} // no-op span
	}
	span, _, err := t.tracer.CreateExitSpanWithContext(ctx, operationName, "grpc-server", func(_, _ string) error {
		return nil // no-op injector (not propagating to downstream)
	})
	if err != nil || span == nil {
		return ctx, &Go2SkySpan{}
	}
	return ctx, &Go2SkySpan{span: span}
}