"""端到端验证 3 个 AI 模型服务（Stage 22-A.4）。

用法：
    python scripts/verify_ai.py                        # 默认 localhost
    python scripts/verify_ai.py --host 192.168.1.10    # 远程

成功标准：所有服务返回 HTTP 200 且 /health 报 status=ok。
"""
import argparse
import json
import sys
import urllib.request
import urllib.error
from dataclasses import dataclass


@dataclass
class Target:
    name: str
    port: int
    health_path: str = "/health"
    metrics_path: str = "/metrics"


TARGETS = [
    Target("fer",       8004),
    Target("sensevoice", 8002),
    Target("xtts",      8003),
]


def http_get(url: str, timeout: float = 5.0) -> tuple[int, dict | str]:
    """Lightweight urllib GET that returns (status_code, parsed_json_or_text)."""
    req = urllib.request.Request(url, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = resp.read()
            try:
                return resp.status, json.loads(data)
            except json.JSONDecodeError:
                return resp.status, data.decode(errors="replace")
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, body
    except (urllib.error.URLError, TimeoutError, OSError) as e:
        return 0, f"connection error: {e}"


def verify(host: str, skip_metrics: bool = False, verbose: bool = True) -> int:
    results = []
    for t in TARGETS:
        url = f"http://{host}:{t.port}{t.health_path}"
        status, body = http_get(url)
        ok = status == 200 and (isinstance(body, dict) and body.get("status") == "ok")
        backend = body.get("backend") or body.get("device") or "unknown" if isinstance(body, dict) else "n/a"
        results.append((t.name, ok, status, backend, body))
        if verbose:
            tag = "[OK  ]" if ok else "[FAIL]"
            print(f"{tag} {t.name:11s} HTTP :{t.port}/health: status={status} backend={backend}")

        if not skip_metrics:
            metrics_status, _ = http_get(f"http://{host}:{t.port}{t.metrics_path}")
            if verbose:
                mtag = "   [OK  ]" if metrics_status == 200 else "   [FAIL]"
                print(f"{mtag} {t.name:11s} HTTP :{t.port}/metrics: status={metrics_status}")

    if verbose:
        passed = sum(1 for r in results if r[1])
        total = len(results)
        print(f"\n=== Summary: {passed}/{total} AI services healthy ===")

    return 0 if all(r[1] for r in results) else 1


def main() -> int:
    parser = argparse.ArgumentParser(description="verify AI model services")
    parser.add_argument("--host", default="localhost", help="host (default: localhost)")
    parser.add_argument("--skip-metrics", action="store_true", help="skip /metrics check")
    args = parser.parse_args()
    return verify(args.host, skip_metrics=args.skip_metrics)


if __name__ == "__main__":
    sys.exit(main())
