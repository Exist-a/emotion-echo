#!/usr/bin/env python3
"""E2E-22 dev-only 告警接收器（mock webhook target）。

存在理由:  Alertmanager 的"通知送达"此前无法被任何断言验证 —— dev 侧只有
`dev-ui` 空 receiver（E2E-F-11），k8s 侧指向不可达的 `.invalid` 占位域名。
于是"告警能响"这件事只能靠人眼看 UI，smoke 永远抓不到"规则加载了但通知
发不出去"这类故障。

本进程接收 Alertmanager 的 webhook POST，把 payload 完整留存，供
`GET /received` 取回，由 scripts/smoke_observability.py 断言:
  - 某条告警**真的送达**（而不是只"加载了规则"）
  - 送达的 body 含 labels / annotations / startsAt（不只是 HTTP 200）
  - 告警**解除**时也送达（status=resolved），验证 send_resolved 链路

仅用于本地 dev 的 obs profile，不含任何鉴权 —— 端口只绑到本机回环语义。
不要在 prod 部署本服务。

端点:
  POST /alert    接收 Alertmanager 通知（也可 POST / 兜底）
  GET  /received 返回最近 N 条通知的 JSON 数组（?limit=，默认 50）
  GET  /health   存活探针
  GET  /reset    清空已收通知（供测试隔离）
"""

import json
import os
import threading
from collections import deque
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(os.environ.get("MOCK_RECEIVER_PORT", "8080"))
DATA_DIR = os.environ.get("MOCK_RECEIVER_DATA_DIR", "/data")
MAX_RECORDS = 500

_lock = threading.Lock()
_records = deque(maxlen=MAX_RECORDS)
_seq = 0


def _persist(record):
    """落盘一份 jsonl，便于容器重启后仍可回查（宿主机 ./tmp 目录）。"""
    try:
        os.makedirs(DATA_DIR, exist_ok=True)
        with open(os.path.join(DATA_DIR, "received.jsonl"), "a", encoding="utf-8") as fh:
            fh.write(json.dumps(record, ensure_ascii=False) + "\n")
    except OSError:
        # 落盘失败不影响主流程（内存态仍可用于断言）
        pass


class Handler(BaseHTTPRequestHandler):
    def _reply(self, code, payload):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        global _seq
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""

        try:
            alert = json.loads(raw.decode("utf-8"))
        except (json.JSONDecodeError, UnicodeDecodeError) as exc:
            self._reply(400, {"error": f"undecodable payload: {exc}"})
            return

        with _lock:
            _seq += 1
            record = {
                "seq": _seq,
                "receivedAt": self.date_time_string(),
                "path": self.path,
                "contentType": self.headers.get("Content-Type", ""),
                "alert": alert,
            }
            _records.append(record)

        _persist(record)
        self._reply(200, {"ok": True, "seq": _seq})

    def do_GET(self):
        if self.path.startswith("/received"):
            query = self.path.partition("?")[2]
            limit = MAX_RECORDS
            for kv in query.split("&"):
                if kv.startswith("limit="):
                    try:
                        limit = max(1, min(MAX_RECORDS, int(kv.split("=", 1)[1])))
                    except ValueError:
                        pass
            with _lock:
                items = list(_records)[-limit:]
            self._reply(200, {"count": len(items), "records": items})
            return

        if self.path.startswith("/health"):
            with _lock:
                total = len(_records)
            self._reply(200, {"ok": True, "received": total})
            return

        if self.path.startswith("/reset"):
            with _lock:
                _records.clear()
            self._reply(200, {"ok": True})
            return

        self._reply(404, {"error": "not found", "path": self.path})

    def log_message(self, fmt, *args):
        # 覆盖默认 stderr 噪声：只有告警投递才值得留痕
        print("[obs-mock-receiver] " + (fmt % args), flush=True)


if __name__ == "__main__":
    print(f"[obs-mock-receiver] listening on :{PORT} (data dir {DATA_DIR})", flush=True)
    HTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
