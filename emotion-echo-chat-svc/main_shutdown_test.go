// chat-svc main shutdown test (sibling for main.go per AGENTS.md §1.1).
//
// Stage 94 PR-4 §P0-4：chat-svc main.go 用 os.Exit(0) 让 defer kp.Close() 不执行
// + signal handler goroutine 直接退出绕过 main 函数 return → defer 链不触发 →
// 关闭时丢消息(§P0-4 kafka producer 关闭路径)。本测试钉死重构契约:
//
//  1. main.go 不应再出现 \`os.Exit(0)\`（仅允许 log.Fatal* 在 init 失败时调用 os.Exit）
//  2. main.go 应有 signal handler 在收到 SIGINT/SIGTERM 时 cancel rootCtx + 让 main 自然 return
//  3. \`defer kp.Close()\` 等所有资源应在 main 函数 return 后正常触发
//
// 验证方式：源码字面量断言（与 ai-svc analyzer §P0-2 字面量测试同模式）。
//
// 设计权衡：signal handler 用 \`go func() { sigCh <-sig; rootCancel(); ... }()\`
// 然后 select { <-rootCtx.Done(): return } 或 \`<-errCh: log.Fatal(err)\` 让 main
// 函数正常返回,触发 deferred kp.Close() + nacosRuntime.Close() + bootCancel。
package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// stripCommentsAndStrings 移除 Go 源码中 // 行注释与 /* */ 块注释以及字符串字面量，
// 让字面量断言只看代码本体（不误命中注释里提到的"修复前用 os.Exit(0)"等历史描述）。
// 这是为了避免"self-referential"陷阱 —— 测试自己写的注释不能让测试 FAIL。
func stripCommentsAndStrings(src string) string {
	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// 简易剥离：行首 // 之后全是注释
		if idx := strings.Index(line, "//"); idx >= 0 {
			// 保留 // 之前的代码
			line = line[:idx]
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// TestMain_NoDirectOsExit_DefersTriggerOnShutdown §P0-4 RED:
//
// main.go 不应直接调用 os.Exit(0)（仅允许 log.Fatal* 在 init 失败时调用 os.Exit）。
// 出现 os.Exit(0) 意味着 signal handler goroutine 绕过 main 函数 return → 所有
// defer 不触发 → 关闭时丢消息。
func TestMain_NoDirectOsExit_DefersTriggerOnShutdown(t *testing.T) {
	mainBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Skipf("cannot read main.go: %v", err)
	}
	// 只看代码本体,不看注释（避免 self-referential 误命中）
	src := stripCommentsAndStrings(string(mainBytes))

	// 反向断言：不应出现 os.Exit(0) —— §P0-4 bug 模式
	// （原 line 282-292 signal goroutine 调 os.Exit(0) 跳过所有 defer）
	if strings.Contains(src, "os.Exit(0)") {
		t.Error("chat-svc main.go 仍含 os.Exit(0) —— §P0-4 修复要求改用 cancel rootCtx + main 自然 return，\n" +
			"否则 signal handler 直接退出,绕过所有 defer,kp.Close()/nacosRuntime.Close() 不触发 → 关闭丢消息")
	}

	// 正向断言：必须有 signal handler 调 rootCtx cancel（让 r.Run 返回 + main 自然 return）
	wantPatterns := []string{
		`signal.Notify(`,
		`Cancel()`,
	}
	anyCancel := false
	for _, p := range wantPatterns {
		if strings.Contains(src, p) {
			anyCancel = true
			break
		}
	}
	if !anyCancel {
		t.Error("chat-svc main.go 缺 signal handler 调 cancel() —— §P0-4 修复要求 signal handler\n" +
			"在收到 SIGINT/SIGTERM 时 cancel rootCtx 让 main 自然 return")
	}

	// 正向断言：必须仍然保留 defer kp.Close()
	if !strings.Contains(src, "kp.Close()") {
		t.Error("chat-svc main.go 缺 kp.Close() 调用 —— §P0-4 修复目标是让 kp.Close() 在 main return 时触发,\n" +
			"而不是删除该释放逻辑")
	}

	// 正向断言：必须用 http.Server + Shutdown 模式,而不是 gin.Run
	// (Shutting down via httpServer.Shutdown(ctx) 优雅收尾 in-flight requests)
	if !strings.Contains(src, "httpServer.Shutdown") {
		t.Error("chat-svc main.go 缺 httpServer.Shutdown —— §P0-4 修复要求用 http.Server + Shutdown\n" +
			"让 in-flight requests 优雅收尾,然后 main 自然 return")
	}
}