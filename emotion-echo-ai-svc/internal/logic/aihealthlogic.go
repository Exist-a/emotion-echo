// Stage 23-C: AI 模型服务集群健康检查
//
// 探测 3 个 AI 服务（FER / SenseVoice / XTTS）的可达性。
// 用于 ARMS / SLO 监控 + 启动期探活。

package logic

import (
	"context"
	"strconv"
	"sync"
	"time"

	"emotion-echo-ai-svc/internal/svc"
)

type AIHealthResp struct {
	Time int64          `json:"time"`
	All  bool           `json:"allHealthy"`
	FER  *AIHealthEntry `json:"fer,omitempty"`
	SV   *AIHealthEntry `json:"sensevoice,omitempty"`
	TTS  *AIHealthEntry `json:"xtts,omitempty"`
}

type AIHealthEntry struct {
	Enabled   bool   `json:"enabled"`
	Healthy   bool   `json:"healthy"`
	Error     string `json:"error,omitempty"`
	URL       string `json:"url,omitempty"`
	LatencyMS string `json:"latencyMs,omitempty"`
}

type AIHealthLogic struct {
	svcCtx *svc.ServiceContext
}

func NewAIHealthLogic(svcCtx *svc.ServiceContext) *AIHealthLogic {
	return &AIHealthLogic{svcCtx: svcCtx}
}

// probeInner 单个服务探活
func probeInner(ctx context.Context, enabled bool, url string, fn func(context.Context) error) *AIHealthEntry {
	e := &AIHealthEntry{Enabled: enabled, URL: url}
	if !enabled {
		e.Healthy = false
		e.Error = "disabled (BaseURL empty)"
		return e
	}
	start := time.Now()
	err := fn(ctx)
	elapsed := time.Since(start)
	e.LatencyMS = strconv.FormatInt(elapsed.Milliseconds(), 10)
	if err != nil {
		e.Healthy = false
		e.Error = err.Error()
		return e
	}
	e.Healthy = true
	return e
}

// Health 并行探活 3 个 AI 服务，整体 timeout 6s
func (l *AIHealthLogic) Health(ctx context.Context) (*AIHealthResp, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	out := &AIHealthResp{Time: time.Now().UnixMilli()}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	setEntry := func(e *AIHealthEntry) {
		mu.Lock()
		// caller assigns to right field; we use the pointer identity
		mu.Unlock()
	}

	_ = setEntry // (we use direct field assignment instead)

	// 并行探活：3 个 goroutine 同时连，每个最多 6s 总超时
	wg.Add(3)
	go func() {
		defer wg.Done()
		out.FER = probeInner(ctx, l.svcCtx.FER != nil, l.svcCtx.Config.FER.BaseURL,
			func(c context.Context) error { return l.svcCtx.FER.Health(c) })
	}()
	go func() {
		defer wg.Done()
		out.SV = probeInner(ctx, l.svcCtx.SenseVoice != nil, l.svcCtx.Config.SenseVoice.BaseURL,
			func(c context.Context) error { return l.svcCtx.SenseVoice.Health(c) })
	}()
	go func() {
		defer wg.Done()
		out.TTS = probeInner(ctx, l.svcCtx.XTTS != nil, l.svcCtx.Config.XTTS.BaseURL,
			func(c context.Context) error { return l.svcCtx.XTTS.Health(c) })
	}()
	wg.Wait()

	out.All = out.FER.Healthy && out.SV.Healthy && out.TTS.Healthy
	return out, nil
}