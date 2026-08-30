// Package sse — encoder.go
//
// Stage 30 / stage-30-web-bff.md T3.25-26: SSE（Server-Sent Events）编码器。
//
// SSE 规范（https://html.spec.whatwg.org/multipage/server-sent-events.html）：
//   每条消息由若干 field 行组成，空行结束：
//     event: <name>          ← 事件名（前端 addEventListener 监听）
//     data: <json>           ← 数据（可多行 data:，浏览器拼接）
//     id: <string>           ← 事件 ID（Last-Event-ID 重连用）
//     retry: <ms>            ← 重连间隔
//
// 编码保证：每条事件末尾留一个空行（浏览器按空行切分事件）。
package sse

import (
	"encoding/json"
	"fmt"
	"io"
)

// Event 是单条 SSE 消息
type Event struct {
	// ID 可选：事件 ID（Last-Event-ID 重连恢复）
	ID string
	// Name 可选：事件名（前端 addEventListener(name)）。空 = message 事件。
	Name string
	// Data 必填：序列化为 JSON 后写入 data: 行
	Data any
	// Retry 可选：重连间隔（毫秒）。0 = 不发送 retry 行。
	Retry int
}

// Encode 把一条事件写入 w（以空行结束）。
//
// data 序列化为单行 JSON（紧凑），避免多行 data: 拼接歧义。
func Encode(w io.Writer, evt Event) error {
	if evt.Name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", evt.Name); err != nil {
			return err
		}
	}
	if evt.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", evt.ID); err != nil {
			return err
		}
	}
	if evt.Retry > 0 {
		if _, err := fmt.Fprintf(w, "retry: %d\n", evt.Retry); err != nil {
			return err
		}
	}
	if evt.Data != nil {
		payload, err := json.Marshal(evt.Data)
		if err != nil {
			return fmt.Errorf("sse: marshal data: %w", err)
		}
		if _, err := fmt.Fprintf(w, "data: %s\n", payload); err != nil {
			return err
		}
	}
	// 空行结束事件
	_, err := io.WriteString(w, "\n")
	return err
}

// EncodeRaw 直接写原始 SSE 文本（供已经序列化的 data 使用）。
func EncodeRaw(w io.Writer, name, id, dataLine string) error {
	if name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", dataLine); err != nil {
		return err
	}
	return nil
}
