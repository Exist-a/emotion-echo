package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 A 组 · 测试点 #7：chat-svc 的 status 判定。
//
// 与其余四个服务不同，chat-svc 的 status 计算**本来就是正确的**
//（healthlogic.go:45-48 按 dbOK || kafkaOK 计算）—— 本测试的作用不是修缺陷，
// 而是**锁住这个正确行为**，防止将来重构时退化成"永远 ok"。
//
// 同时如实记录一条已知局限：kafkaOK 只判 EventPublisher 非 nil，
// **不真连 Kafka**（plan §2 A2 / 测试点 #7 的"二选一"尚未落地，
// 执行期须补真实探测或明确写进文档）。

// pingFailConversationRepo 嵌入 InMemory，只覆写 Ping。
type pingFailConversationRepo struct {
	*repository.InMemoryConversationRepo
	pingErr error
}

func (f *pingFailConversationRepo) Ping(ctx context.Context) error { return f.pingErr }

var _ repository.ConversationRepo = (*pingFailConversationRepo)(nil)

// stubPublisher 是只满足接口形状的最小替身：健康检查只判它是否为 nil，
// 不实际调用 Publish（若调用应立刻暴露）。
type stubPublisher struct{}

func (stubPublisher) Publish(ctx context.Context, topic string, e *events.Event) error {
	panic("健康检查测试不应调用 Publish")
}
func (stubPublisher) Close() error { panic("健康检查测试不应调用 Close") }

var _ events.EventPublisher = stubPublisher{}

func newStubPublisher() events.EventPublisher { return stubPublisher{} }

// TestHealthLogic_Health_StatusReflectsDependencies 锁住既有正确行为。
func TestHealthLogic_Health_StatusReflectsDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pingErr     error
		withPub     bool
		wantStatus  string
		wantDbOK    bool
		wantKafkaOK bool
	}{
		{
			name:        "all dependencies available returns ok",
			pingErr:     nil,
			withPub:     true,
			wantStatus:  "ok",
			wantDbOK:    true,
			wantKafkaOK: true,
		},
		{
			name:        "db down returns degraded",
			pingErr:     errors.New("connection refused"),
			withPub:     true,
			wantStatus:  "degraded",
			wantDbOK:    false,
			wantKafkaOK: true,
		},
		{
			name:        "kafka publisher absent returns degraded",
			pingErr:     nil,
			withPub:     false,
			wantStatus:  "degraded",
			wantDbOK:    true,
			wantKafkaOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &pingFailConversationRepo{
				InMemoryConversationRepo: repository.NewInMemoryConversationRepo(),
				pingErr:                  tt.pingErr,
			}
			svcCtx := &svc.ServiceContext{Config: config.Config{}, ConversationRepo: repo}
			if tt.withPub {
				svcCtx.EventPublisher = newStubPublisher()
			}

			l := NewHealthLogic(context.Background(), svcCtx)
			resp, err := l.Health()

			require.NoError(t, err, "依赖不可达不应是 handler 级错误，应由 status 表达")
			assert.Equal(t, tt.wantStatus, resp.Status)
			assert.Equal(t, tt.wantDbOK, resp.DbOK)
			assert.Equal(t, tt.wantKafkaOK, resp.KafkaOK)
		})
	}
}

// TestHealthLogic_Health_NilRepoAndPublisher 边界：两个依赖都缺时，**两个都要报不健康**。
//
// 2026-09-30 契约反转（E2E-23 F-96 实测驱动）：原断言是
// `DbOK == true`，注释「repo 未注入时 DbOK 保持 true（不谎报失败）」。
// 该推理把"这个部署本来不接 DB"与"main.go 单次连接失败后的降级启动"混为一谈 ——
// 而后者在生产中真实发生，且会让 `/health/ready` 返 200、容器判 healthy、
// APISIX 照常路由而后端全挂，**零告警**（实测见 E2E-23 report）。
func TestHealthLogic_Health_NilRepoAndPublisher(t *testing.T) {
	t.Parallel()

	l := NewHealthLogic(context.Background(), &svc.ServiceContext{Config: config.Config{}})
	resp, err := l.Health()

	require.NoError(t, err)
	assert.Equal(t, "degraded", resp.Status,
		"Kafka publisher 缺失必须报 degraded —— 否则事件永远发不出去而探针说健康")
	assert.False(t, resp.DbOK,
		"repo 为 nil（生产降级启动的真实形态）时 DbOK 必须是 false —— "+
			"数据库层不存在，不存在「依赖正常」这回事")
	assert.False(t, resp.KafkaOK)
}
