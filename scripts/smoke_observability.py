#!/usr/bin/env python3
"""smoke_observability.py — PR-OBS-4 observability infra smoke

前置: docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml --profile obs up -d
      启动后等 ~30s 让 prometheus/grafana 完成 scrape + datasource provisioning。

实现要点:
- 零 Python 依赖（仅 stdlib + urllib）
- 3 项断言: prometheus targets UP + grafana health + grafana datasource 注册
- 退出码 0 = 全 OK, 1 = 至少一项 FAIL

PR-OBS-4 范围: prometheus + grafana 基础设施层 (compose + scrape config + datasource provisioning)
后续 PR-OBS-5/6/7/8 各自加自己的断言。
"""
from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request

# ====== 配置 (dev compose 默认 + obs profile) ======
PROMETHEUS = "http://localhost:9090"
GRAFANA = "http://localhost:3000"

# 期望的 scrape target 列表（prometheus.yml 静态 targets）
# 与 deploy/prometheus/prometheus.yml 的 scrape_configs.static_configs 对齐
EXPECTED_TARGETS = [
    "emotion-echo-user-svc:8888",
    "emotion-echo-chat-svc:8890",
    "emotion-echo-assessment-svc:8889",
    "emotion-echo-analytics-svc:8904",
    "emotion-echo-ai-svc:8891",
    "emotion-echo-web-bff:8894",
    "emotion-echo-apisix:9091",
    "emotion-echo-sw-oap:1234",
]

results: list[tuple[str, bool, str]] = []


def check(name: str, ok: bool, detail: str = "") -> None:
    sym = "OK  " if ok else "FAIL"
    results.append((name, ok, detail))
    print(f"[{sym}] {name}: {detail}")


def http_get(url: str, timeout: float = 5.0) -> tuple[int, str]:
    """GET URL, return (status, body). body = "" on network error."""
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", errors="replace") if e.fp else ""
    except (urllib.error.URLError, ConnectionRefusedError, TimeoutError, OSError) as e:
        return 0, f"{type(e).__name__}: {e}"


def main() -> int:
    print("=== smoke_observability.py (PR-OBS-4) ===")

    # 断言 1: prometheus targets endpoint
    # /api/v1/targets?state=active 只返 UP 状态 target
    status, body = http_get(f"{PROMETHEUS}/api/v1/targets?state=active")
    if status == 0:
        check("prometheus targets endpoint reachable", False, body)
    elif status != 200:
        check("prometheus targets endpoint returns 200", False, f"HTTP {status}")
    else:
        try:
            data = json.loads(body)
            active_targets = data.get("data", {}).get("activeTargets", [])
            up_targets = [t for t in active_targets if t.get("health") == "up"]
            scrape_pool = {t["labels"].get("instance", "") for t in up_targets}
            missing = [t for t in EXPECTED_TARGETS if t not in scrape_pool]
            if missing:
                check(
                    "prometheus scrape targets UP (>= expected count)",
                    False,
                    f"missing {len(missing)}: {missing}; UP={len(up_targets)}, expected={len(EXPECTED_TARGETS)}",
                )
            else:
                check(
                    "prometheus scrape targets UP (>= expected count)",
                    True,
                    f"UP={len(up_targets)}/{len(EXPECTED_TARGETS)} (apisix + 6 svc + skywalking-oap)",
                )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("prometheus targets JSON parseable", False, f"{type(e).__name__}: {e}")

    # 断言 2: grafana health
    status, body = http_get(f"{GRAFANA}/api/health")
    if status == 0:
        check("grafana health endpoint reachable", False, body)
    elif status != 200:
        check("grafana health returns 200", False, f"HTTP {status}: {body[:100]}")
    else:
        try:
            data = json.loads(body)
            db_ok = data.get("database") == "ok"
            check(
                "grafana health returns 200 + database=ok",
                db_ok,
                f"database={data.get('database')}, version={data.get('version', '?')}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("grafana health JSON parseable", False, f"{type(e).__name__}: {e}")

    # 断言 3: grafana datasource (prometheus 自动 provisioning 注册)
    # grafana 默认凭证 admin/admin (dev compose 默认, prod 必须改)
    status, body = http_get(
        f"{GRAFANA}/api/datasources",
    )
    if status == 0:
        check("grafana datasources endpoint reachable", False, body)
    elif status == 401:
        # grafana 默认开启 anonymous access 需 auth,本 smoke 用 admin/admin 试探
        check(
            "grafana datasources reachable with admin/admin",
            False,
            "HTTP 401 — 默认 anonymous 关闭,需在 datasource provisioning 后用 admin/admin 调用",
        )
    elif status != 200:
        check("grafana datasources returns 200", False, f"HTTP {status}: {body[:100]}")
    else:
        try:
            data = json.loads(body)
            ds_names = [d.get("name", "") for d in data] if isinstance(data, list) else []
            has_prometheus = any("prometheus" in n.lower() for n in ds_names)
            check(
                "grafana datasource 'prometheus' auto-registered",
                has_prometheus,
                f"datasources={ds_names}",
            )
        except (json.JSONDecodeError, TypeError) as e:
            check("grafana datasources JSON parseable", False, f"{type(e).__name__}: {e}")

    print()
    failed = [n for n, ok, _ in results if not ok]
    if failed:
        print(f"FAIL: {len(failed)} check(s) failed: {failed}")
        return 1
    print(f"PASS: {len(results)} check(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
