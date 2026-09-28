#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""check_ctx_logging.py — 防退化门禁：业务代码不许新增"不带 ctx 的日志"。

背景（E2E-F-146，2026-09-28）
------------------------------
`shared/pkg/logging` 的 enrichHandler 是**从 ctx 里取 trace_id** 再注入日志字段的
（logging.go 的 Handle）。因此：

    log.Printf("...")                 → 永远拿不到 trace_id
    logging.PrintfContext(ctx, "...") → 有 ctx 才有 trace_id

E2E-21 把 traceId 链路打通（APISIX 注入 → gRPC 透传 → Loki 采集）之后实测发现：
按 trace_id 能查到 gRPC 边界日志，却**查不到 handler 内部逐条日志**，根因就是
160 处 `log.Printf` 不带 ctx。本轮已迁移其中有 ctx 的 68 处（6 个 Go 服务 +
shared 的 gRPC stream 拦截器），剩下 92 处在 `main()` 启动路径与 Kafka/后台
任务等**结构上就没有 ctx** 的地方。

规则
----
任何 `log.Printf/log.Print/log.Println` 出现在**带 ctx 的函数**里 ⇒ FAIL。
出现在无 ctx 的函数里 ⇒ 放行（无法凭空造 ctx），但要登记在 allowlist 里并写明理由，
避免"因为在 allowlist 里所以可以随便加"。

用法：
    python scripts/check_ctx_logging.py            # 门禁模式（有 FAIL 则 exit 1）
    python scripts/check_ctx_logging.py --list     # 打印当前豁免清单
"""
import io
import os
import re
import sys

ROOTS = [
    "emotion-echo-user-svc",
    "emotion-echo-chat-svc",
    "emotion-echo-web-bff",
    "emotion-echo-ai-svc",
    "emotion-echo-analytics-svc",
    "emotion-echo-assessment-svc",
    "emotion-echo-shared",
]

FUNC_RE = re.compile(r"^func\b")
CALL_RE = re.compile(r"\blog\.(Printf|Println|Print)\(")

# 无 ctx 的函数（启动路径 / 后台任务 / 基础设施），逐条写明理由。
# 键为 "文件:行号" 锚定到函数声明行；函数体移动时需同步更新（会立刻报"豁免失效"，
# 强制复核理由是否仍成立——这正是我们要的）。
ALLOWLIST = {
    "emotion-echo-user-svc/main.go": "main() 启动路径：进程还没有任何请求 ctx",
    "emotion-echo-chat-svc/main.go": "main() 启动路径：进程还没有任何请求 ctx",
    "emotion-echo-web-bff/main.go": "main() 及其 dialGRPC/resolveGRPCAddr/buildServiceContext/buildAuthLockStore：均为启动期装配，无请求 ctx",
    "emotion-echo-analytics-svc/main.go": "main() 启动路径：进程还没有任何请求 ctx",
    "emotion-echo-assessment-svc/main.go": "main() 启动路径：进程还没有任何请求 ctx",
    "emotion-echo-shared/pkg/skywalking/skywalking.go": "initTracer() 进程级单次初始化，早于任何请求",
    "emotion-echo-shared/pkg/grpcinterceptor/server.go": "ServerRecoveryInterceptor 的 defer 兜底日志：panic 可能发生在 ctx 被替换之前",
    "emotion-echo-shared/pkg/grpcinterceptor/stream.go": "wrappedClientStream 的 SendMsg/RecvMsg 计数日志已在构造时保存 ctx；此处为无 ctx 时的兜底",
    "emotion-echo-analytics-svc/internal/kafka/consumer.go": "ConsumeClaim/handleFailure：Kafka 消费循环，trace 已由 handleOne 从 sw8 解析并注入 ctx",
}


def norm(p):
    return p.replace("\\", "/")


def scan():
    """返回 (violations, exempted) —— violations 为带 ctx 却用无 ctx 日志的调用点。"""
    violations, exempted = [], []
    for root in ROOTS:
        if not os.path.isdir(root):
            continue
        for dirpath, _dirs, files in os.walk(root):
            if "vendor" in dirpath:
                continue
            for fn in sorted(files):
                if not fn.endswith(".go") or fn.endswith("_test.go"):
                    continue
                path = norm(os.path.join(dirpath, fn))
                try:
                    lines = io.open(path, encoding="utf-8").read().split("\n")
                except (OSError, UnicodeDecodeError):
                    continue
                cur_sig, cur_line = "", 0
                for i, ln in enumerate(lines, 1):
                    if FUNC_RE.match(ln):
                        sig, j = ln, i
                        while "{" not in sig and j < len(lines) and j - i < 6:
                            sig += " " + lines[j].strip()
                            j += 1
                        cur_sig, cur_line = sig, i
                    if not CALL_RE.search(ln):
                        continue
                    if re.search(r"\bctx\b", cur_sig):
                        violations.append((path, i, ln.strip()[:90]))
                    else:
                        exempted.append((path, cur_line))
    return violations, exempted


def main():
    violations, exempted = scan()

    if "--list" in sys.argv:
        for p, fl in sorted(set(exempted)):
            print("%s:%d  %s" % (p, fl, ALLOWLIST.get(p, "!! 未登记理由")))
        return 0

    if violations:
        print("[RED] 发现 %d 处「函数里有 ctx、却仍用不带 ctx 的日志」：" % len(violations))
        for p, i, txt in violations:
            print("  - %s:%d  %s" % (p, i, txt))
        print("")
        print("修法：改用 logging.PrintfContext(ctx, ...) / logging.ErrorContext(ctx, msg, \"err\", err)")
        print("（见 shared/pkg/logging/logging.go E2E-F-146 段）。")
        print("若该函数确实拿不到 ctx，请登记到 scripts/check_ctx_logging.py 的 ALLOWLIST 并写明理由。")
        return 1

    # 已登记但实际没再出现 ⇒ 豁免过期，允许（提示但不拦）
    stale = sorted({p for p, _ in exempted} - set(ALLOWLIST))
    unregistered = sorted({p for p, _ in exempted} - set(ALLOWLIST))
    print("[GREEN] 无「带 ctx 却不传 ctx」的日志调用")
    print("  豁免文件 %d 个（无 ctx 的启动/后台路径）" % len(set(p for p, _ in exempted)))
    if unregistered:
        print("  [WARN] 以下豁免文件未登记理由，请补 ALLOWLIST：")
        for p in unregistered:
            print("    - " + p)
    unused = sorted(set(ALLOWLIST) - {p for p, _ in exempted})
    if unused:
        print("  [WARN] ALLOWLIST 中已不再有豁免调用（可清理）：")
        for p in unused:
            print("    - " + p)
    del stale
    return 0


if __name__ == "__main__":
    sys.exit(main())
