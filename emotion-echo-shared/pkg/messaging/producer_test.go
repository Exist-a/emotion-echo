package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🔴 第一轮 RED：定义"应有的行为"，代码还没写
//
// Producer 必须满足：
//   - Publish(ctx, msg) 在 broker 不可达时返回 error
//   - Publish 接受 context 来控制超时/取消
//   - Close() 关闭后再次 Publish 应返回 ErrProducerClosed
//
// 这里没有真实 Kafka，in-memory mock 即可验证接口语义

func TestProducer_Publish_ValidatesInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     Message
		wantErr error
	}{
		{
			name:    "空 Topic 必须报错",
			msg:     Message{Topic: "", Value: []byte("hello")},
			wantErr: ErrEmptyTopic,
		},
		{
			name:    "空 Value 必须报错",
			msg:     Message{Topic: "users", Value: nil},
			wantErr: ErrEmptyValue,
		},
		{
			name:    "正常消息 OK",
			msg:     Message{Topic: "users", Value: []byte("hello")},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 用 in-memory producer，不需要真实 broker
			p := NewInMemoryProducer()
			defer p.Close()

			err := p.Publish(context.Background(), tc.msg)
			if tc.wantErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestProducer_Publish_RespectsContextCancellation(t *testing.T) {
	t.Parallel()
	t.Run("传已被 cancel 的 ctx 必须立即返回", func(t *testing.T) {
		t.Parallel()

		p := NewInMemoryProducer()
		defer p.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立刻取消

		err := p.Publish(ctx, Message{Topic: "users", Value: []byte("hi")})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("传已超时的 ctx 必须返回 DeadlineExceeded", func(t *testing.T) {
		t.Parallel()

		p := NewInMemoryProducer()
		defer p.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// 让 mock 的 publish 比 ctx 更慢
		time.Sleep(5 * time.Millisecond)
		err := p.Publish(ctx, Message{Topic: "users", Value: []byte("hi")})
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded))
	})
}

func TestProducer_Close_PreventsFurtherPublish(t *testing.T) {
	t.Parallel()

	p := NewInMemoryProducer()
	require.NoError(t, p.Close())

	err := p.Publish(context.Background(), Message{Topic: "users", Value: []byte("hi")})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProducerClosed)
}

func TestInMemoryProducer_StoresMessagesInOrder(t *testing.T) {
	t.Parallel()

	p := NewInMemoryProducer()
	defer p.Close()

	want := []string{"m1", "m2", "m3"}
	for _, v := range want {
		require.NoError(t, p.Publish(context.Background(), Message{Topic: "q", Value: []byte(v)}))
	}

	got := p.Drain()
	require.Len(t, got, 3)
	for i, v := range want {
		assert.Equal(t, v, string(got[i].Value))
	}
}
