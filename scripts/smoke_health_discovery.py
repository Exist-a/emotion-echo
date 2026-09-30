#!/usr/bin/env python3
"""smoke_health_discovery.py —— E2E-23 编排级健康与服务发现事实核验

为什么需要这个脚本（而不是只靠 Playwright）：

1. **探针视角**：6 个 Go 服务里只有 BFF(8894) 与 ai-svc(8892，且是 gRPC 口) 把
   端口映射到宿主，其余 4 个服务在宿主侧**直连返 000**（plan §0 F-k）。
   本脚本走 `docker exec` 进容器网络内探，才是真实可达的视角。
2. **破坏性验证**：readiness 的"依赖挂了返 503"只能在**停掉依赖**时观察。
   这属破坏性操作，不该塞进 Playwright（且会让 CI 抖动），由本脚本在明确
   标注为可选的模式下执行。
3. **CI 可执行**：纯 HTTP + docker exec，无 Playwright 依赖。

用法：
    python scripts/smoke_health_discovery.py              # 只读检查
    python scripts/smoke_health_discovery.py --with-chaos # 额外做"停依赖验降级"（破坏性）
    python scripts/smoke_health_discovery.py --only-bff   # 只查 BFF（宿主直连模式）

退出码：0 全绿 / 1 有失败 / 2 环境不满足（如 docker 不可用）
"""

import argparse
import json
import subprocess
import sys
import urllib.error
import urllib.request

# 容器名 → (容器内端口, 探针路径)
SERVICES = {
    "emotion-echo-user-svc": 8888,
    "emotion-echo-chat-svc": 8890,
    "emotion-echo-analytics-svc": 8893,
    "emotion-echo-assessment-svc": 8889,
    "emotion-echo-ai-svc": 8891,
    "emotion-echo-web-bff": 8894,
}

NACOS_LIST = (
    "http://localhost:8848/nacos/v1/ns/service/list"
    "?pageNo=1&pageSize=50&namespaceId=emotion-echo-dev"
)
EXPECTED_NACOS = 6

results = []


def check(name: str, ok: bool, detail: str = "") -> None:
    results.append((name, ok, detail))
    print(f"[{'OK  ' if ok else 'FAIL'}] {name}: {detail}")


def http_get(url: str, timeout: float = 5.0) -> tuple[int, str]:
    """GET URL，返回 (status, body)；网络错误时 status=0。"""
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", errors="replace") if e.fp else ""
    except (urllib.error.URLError, ConnectionRefusedError, TimeoutError, OSError) as e:
        return 0, f"{type(e).__name__}: {e}"


def docker(*args: str, timeout: float = 20.0) -> tuple[int, str, str]:
    """执行 docker 命令，返回 (rc, stdout, stderr)。"""
    try:
        p = subprocess.run(
            ["docker", *args], capture_output=True, text=True, timeout=timeout
        )
        return p.returncode, p.stdout, p.stderr
    except (OSError, subprocess.SubprocessError) as e:
        return 127, "", f"{type(e).__name__}: {e}"


def probe_in_container(svc: str, port: int, path: str) -> tuple[int, str]:
    """在容器网络内探测（绕开宿主端口未映射的限制）。"""
    rc, out, err = docker("exec", svc, "wget", "-qO-", "--timeout=5", f"http://127.0.0.1:{port}{path}")
    if rc != 0:
        return rc, (err or out).strip()[:200]
    return 0, out.strip()


def check_liveness() -> None:
    """每个服务的 /health：恒 200 + status 字段说真话。"""
    print("\n--- /health（liveness：恒 200）---")
    for svc, port in SERVICES.items():
        rc, body = probe_in_container(svc, port, "/health")
        if rc != 0:
            check(f"{svc} /health", False, f"探测失败 rc={rc}: {body}")
            continue
        try:
            data = json.loads(body)
        except json.JSONDecodeError as e:
            check(f"{svc} /health", False, f"响应非合法 JSON: {e} / {body[:120]}")
            continue
        # 关键断言：status 与 dbOk 不得自相矛盾（F-a 缺陷形态）
        contradiction = data.get("dbOk") is False and data.get("status") == "ok"
        check(
            f"{svc} /health",
            not contradiction,
            f"status={data.get('status')} dbOk={data.get('dbOk')}"
            + ("  ← 响应体自相矛盾（依赖挂了却报 ok）" if contradiction else ""),
        )


def check_readiness() -> None:
    """每个服务的 /health/ready：依赖正常时 200。"""
    print("\n--- /health/ready（readiness：正常态 200）---")
    for svc, port in SERVICES.items():
        rc, out, _ = docker(
            "exec", svc, "sh", "-c",
            f"wget -qO- --timeout=5 http://127.0.0.1:{port}/health/ready",
        )
        if rc != 0:
            check(f"{svc} /health/ready", False, f"端点不存在或不可达 rc={rc}（镜像可能未重建）")
            continue
        try:
            data = json.loads(out.strip())
        except json.JSONDecodeError:
            check(f"{svc} /health/ready", False, f"响应非 JSON: {out[:120]}")
            continue
        check(
            f"{svc} /health/ready",
            data.get("status") == "ok",
            f"status={data.get('status')}",
        )


def check_nacos() -> None:
    """Nacos 注册齐全性。"""
    print("\n--- Nacos 服务发现 ---")
    code, body = http_get(NACOS_LIST, timeout=8.0)
    if code != 200:
        check("Nacos 服务列表", False, f"HTTP {code}（Nacos 未就绪？）{body[:100]}")
        return
    try:
        doms = json.loads(body).get("doms", [])
    except json.JSONDecodeError as e:
        check("Nacos 服务列表", False, f"响应非 JSON: {e}")
        return
    check(
        f"Nacos 注册数 == {EXPECTED_NACOS}",
        len(doms) == EXPECTED_NACOS,
        f"实际 {len(doms)}: {sorted(doms)}",
    )


def check_gateway() -> None:
    """经网关打一个业务端点，证明 discovery 在消费侧生效。"""
    print("\n--- 网关链路（discovery 消费侧）---")
    code, body = http_get("http://localhost:19080/api/v1/users/me", timeout=10.0)
    check(
        "网关 → user-svc 路由通",
        code == 401,
        f"期望 401（未带凭证），实际 {code} —— 若 502/503 则 APISIX 解析不到节点",
    )


def check_chaos() -> None:
    """破坏性：停 Postgres 验 readiness 真的返 503。

    只验 user-svc（其余服务在 dev 里也依赖 DB，但停 DB 会连锁影响多个
    服务，逐个验成本过高且无额外信息量）。
    """
    print("\n--- [破坏性] 停 Postgres 验降级 ---")
    rc, _, err = docker("stop", "emotion-echo-postgres", timeout=60.0)
    if rc != 0:
        check("停 postgres", False, f"rc={rc}: {err[:150]}")
        return
    print("  （已停 postgres，等待探针生效）")
    try:
        import time

        time.sleep(8)
        rc, out, _ = docker(
            "exec", "emotion-echo-web-bff", "sh", "-c",
            "wget -qO- --timeout=5 http://emotion-echo-user-svc:8888/health/ready",
        )
        # wget 遇 503 返回非零（实测 exit=8），这是"预期行为"
        if rc != 0:
            # 用带状态码输出的方式确认确实是 503 而非连不上
            rc2, out2, _ = docker(
                "exec", "emotion-echo-web-bff", "sh", "-c",
                "wget -S -qO- --timeout=5 http://emotion-echo-user-svc:8888/health/ready 2>&1 | grep -i 'HTTP/' | tail -1",
            )
            has503 = "503" in out2
            check("DB 挂时 user-svc /health/ready 返 503", has503, f"探测输出: {out2.strip()[:120] or out.strip()[:120]}")
        else:
            check(
                "DB 挂时 user-svc /health/ready 返 503",
                False,
                f"仍返 200（响应 {out.strip()[:120]}）—— readiness 没生效",
            )
    finally:
        print("  （恢复 postgres）")
        docker("start", "emotion-echo-postgres", timeout=60.0)
        import time

        time.sleep(20)
        rc, out, _ = docker(
            "exec", "emotion-echo-web-bff", "sh", "-c",
            "wget -qO- --timeout=5 http://emotion-echo-user-svc:8888/health/ready",
        )
        recovered = rc == 0 and '"ok"' in out
        check("恢复后 user-svc /health/ready 回 200", recovered, out.strip()[:120] or f"rc={rc}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--with-chaos", action="store_true", help="额外做停依赖验降级（破坏性）")
    parser.add_argument("--only-bff", action="store_true", help="只查 BFF（宿主直连，供无 docker 环境的 CI）")
    args = parser.parse_args()

    print("=== smoke_health_discovery.py (E2E-23) ===")

    rc, _, err = docker("info", "--format", "{{.ServerVersion}}", timeout=15.0)
    if rc != 0:
        print(f"环境不满足：docker 不可用（rc={rc}）{err[:150]}")
        return 2

    if args.only_bff:
        code, body = http_get("http://localhost:8894/health", timeout=8.0)
        check("BFF /health（宿主直连）", code == 200, f"HTTP {code}: {body[:120]}")
    else:
        check_liveness()
        check_readiness()
        check_nacos()

    check_gateway()

    if args.with_chaos:
        check_chaos()

    print()
    failed = [n for n, ok, _ in results if not ok]
    if failed:
        print(f"FAIL: {len(failed)} check(s) failed: {failed}")
        return 1
    print(f"PASS: {len(results)} check(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
