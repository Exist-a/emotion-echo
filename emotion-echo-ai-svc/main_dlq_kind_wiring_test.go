package main

import (
	"os"
	"strings"
	"testing"
)

// 源契约（CL-03 T-07）：启动日志必须用 `consumer.DLQKind(...)` 报 DLQ 装配种类。
//
// 反例（本次修掉的）：`dlq != nil` —— 生产装配在未配 DLQ 时传
// `consumer.NoopDLQPublisher{}`（**非 nil 的接口值**）⇒ 日志恒打印 `dlq=true`，
// 让人误判"DLQ 已就绪"，正是本阶段反复踩的"假绿信号"。
func TestMain_ConsumerStartedLog_UsesDLQKind(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	s := string(src)

	if !strings.Contains(s, "consumer.DLQKind(") {
		t.Error("main.go 的 consumer started 日志应使用 consumer.DLQKind(...) 报告 DLQ 装配种类（none/noop/kafka）")
	}
	if strings.Contains(s, "dlq != nil") {
		t.Error("main.go 仍在使用 `dlq != nil` 判断 DLQ 是否就绪：Noop 装配（非 nil）下恒 true ⇒ 日志与事实相反")
	}
}
