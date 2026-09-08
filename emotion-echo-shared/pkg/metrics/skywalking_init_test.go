package metrics

import (
	"testing"
)

// PR-OBS-2 RED: SkyWalkingInitFailedTotal counter 应让 7 svc tracer init 失败可被 smoke 抓到
//
// 动机：
//   - 现状：5 svc (chat/assessment/analytics/web-bff/ai-svc) tracer init 静默吞错
//     `tracer, _ = go2sky.NewTracer(...)`,skywalking 不可达时无人能感知
//   - PR-OBS-2 目标：counter emotion_echo_skywalking_init_failed_total{service}
//     让 smoke 脚本 / Prometheus alertmanager 能抓到
//
// 验证：
//   - counter 已注册到 default registry
//   - IncSkyWalkingInitFailed(svc) 调用后 readCounter 返回 1
//   - 多个 svc 各自独立计数

func TestSkyWalkingInitFailedTotal_Inc_SingleService(t *testing.T) {
	const svc = "test-svc-skywalking-inc"

	before := readCounter(t, "emotion_echo_skywalking_init_failed_total", map[string]string{
		"service": svc,
	})

	IncSkyWalkingInitFailed(svc)

	after := readCounter(t, "emotion_echo_skywalking_init_failed_total", map[string]string{
		"service": svc,
	})

	if after-before != 1 {
		t.Errorf("IncSkyWalkingInitFailed(%q): counter delta = %v, want 1", svc, after-before)
	}
}

func TestSkyWalkingInitFailedTotal_Inc_MultipleServicesIndependent(t *testing.T) {
	const svcA = "test-svc-sky-A"
	const svcB = "test-svc-sky-B"

	IncSkyWalkingInitFailed(svcA)
	IncSkyWalkingInitFailed(svcA)
	IncSkyWalkingInitFailed(svcB)

	a := readCounter(t, "emotion_echo_skywalking_init_failed_total", map[string]string{"service": svcA})
	b := readCounter(t, "emotion_echo_skywalking_init_failed_total", map[string]string{"service": svcB})

	// 注：readCounter 返回 0 时无法区分"未注册"与"counter 真为 0",故只断言 delta 关系
	if a < 2 {
		t.Errorf("svcA counter = %v, want >= 2", a)
	}
	if b < 1 {
		t.Errorf("svcB counter = %v, want >= 1", b)
	}
}
