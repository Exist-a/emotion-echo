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
import os
import re
import time
import sys
import urllib.error
import urllib.parse
import urllib.request

# ====== 配置 (dev compose 默认 + obs profile) ======
PROMETHEUS = "http://localhost:9090"
GRAFANA = "http://localhost:13000"  # Stage 74: 宿主 13000（让位 web 前端 :3000）
LOKI = "http://localhost:3100"
# Loki 查询新鲜度窗口（秒）：只认这个窗口内的日志，否则存量数据会掩盖采集中断
FRESH_WINDOW = 300
KAFKA_EXPORTER = "http://localhost:9308"
ALERTMANAGER = "http://localhost:9093"  # Stage 86: alertmanager Web UI

# 期望的 scrape target 列表（prometheus.yml 静态 targets）
# 与 deploy/prometheus/prometheus.yml 的 scrape_configs.static_configs 对齐。
#
# ⚠️ 本清单与 prometheus.yml 是**两份手写副本**，历史上已漂移过一次：
#   llm-service 暴露了 /metrics（emotion-llm-service/main.py:215）却既不在
#   prometheus.yml 也不在本清单 ⇒ 双方都"通过"，缺口结构上无法被发现
#   （2026-09-29 E2E-22 实测修复）。故下方新增"直接从 prometheus.yml 解析比对"的
#   断言组：任何一侧漏登记都会让 smoke 变红，而不是等到指标静默缺失才发现。
#
# O-1 落地: sw-oap env 已加 SW_TELEMETRY=prometheus,OAP 监听 :1234,纳入期望 target。
# E2E-22: 补 llm-service（Python 服务同样暴露 /metrics）。
EXPECTED_TARGETS = [
    "emotion-echo-user-svc:8888",
    "emotion-echo-chat-svc:8890",
    "emotion-echo-assessment-svc:8889",
    "emotion-echo-analytics-svc:8893",
    "emotion-echo-ai-svc:8891",
    "emotion-echo-web-bff:8894",
    "emotion-llm-service:8000",
    "emotion-echo-apisix:9091",
    "emotion-echo-sw-oap:1234",
]

results: list[tuple[str, bool, str]] = []


def check(name: str, ok: bool, detail: str = "") -> None:
    sym = "OK  " if ok else "FAIL"
    results.append((name, ok, detail))
    print(f"[{sym}] {name}: {detail}")


def http_get(url: str, timeout: float = 5.0, headers: dict | None = None) -> tuple[int, str]:
    """GET URL, return (status, body). body = "" on network error."""
    try:
        req = urllib.request.Request(url)
        if headers:
            for k, v in headers.items():
                req.add_header(k, v)
        with urllib.request.urlopen(req, timeout=timeout) as resp:
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

    # ===== PR-OBS-6: Grafana dashboard provisioning 断言 =====

    # 断言 7: emotion-echo-overview 看板 API 返回 200 (用 admin/admin basic auth)
    # grafana dashboard API 需要 Viewer 以上权限,匿名访问 (PR-OBS-4 GF_AUTH_ANONYMOUS_ENABLED=true)
    # 仅对 /api/health 等公开端点开放,dashboard 查询需 login。
    import base64
    grafana_auth = base64.b64encode(b"admin:admin").decode("ascii")
    status, body = http_get(
        f"{GRAFANA}/api/dashboards/uid/emotion-echo-overview",
        headers={"Authorization": f"Basic {grafana_auth}"},
    )
    if status == 0:
        check("grafana dashboard 'emotion-echo-overview' API reachable", False, body)
    elif status != 200:
        check(
            "grafana dashboard 'emotion-echo-overview' returns 200",
            False,
            f"HTTP {status}: {body[:100]}",
        )
    else:
        try:
            data = json.loads(body)
            title = data.get("dashboard", {}).get("title", "?")
            panels = data.get("dashboard", {}).get("panels", [])
            check(
                "grafana dashboard 'emotion-echo-overview' provisioned",
                len(panels) >= 4,
                f"title={title!r}, panels={len(panels)}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("grafana dashboard JSON parseable", False, f"{type(e).__name__}: {e}")

    # ===== E2E-22: 面板"有 JSON" ≠ "有数据" =====
    # 背景: 本 smoke 此前只断言 dashboard JSON 存在 + 面板数 >= 4，**从不查询面板 expr**。
    #   ⇒ 面板引用不存在的指标、或 expr 语义错误（如用 consumer lag 查一个没有常驻
    #   consumer 的 DLQ topic）时，本 smoke 全绿，而 Grafana 上是一屏 "No data"。
    #   这正是 memory `frontend-visual-evidence-failure-modes` 的形态在监控面的翻版。
    # 2026-09-29 实测确认两处: ① 5xx 错误率面板在"零错误"时返回 0 series（用户无法
    #   区分"没有错误"与"面板坏了"）；② DLQ 面板因该 topic 无常驻 consumer 恒 0 series。
    # 本组把每个面板的 expr 真正打到 Prometheus 上，要求返回非空 series。

    def _panel_exprs(uid):
        st, bd = http_get(
            f"{GRAFANA}/api/dashboards/uid/{uid}",
            headers={"Authorization": f"Basic {grafana_auth}"},
        )
        if st != 200:
            return None, f"HTTP {st}"
        try:
            panels = json.loads(bd).get("dashboard", {}).get("panels", [])
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            return None, f"{type(e).__name__}: {e}"
        out = []
        for p in panels:
            for t in p.get("targets", []):
                if t.get("expr"):
                    out.append((p.get("title", "?"), t["expr"]))
        return out, None

    for _uid in ("emotion-echo-overview", "kafka-consumer-lag"):
        _exprs, _err = _panel_exprs(_uid)
        if _exprs is None:
            check(f"grafana dashboard '{_uid}' panels readable", False, _err or "unknown")
            continue
        if not _exprs:
            check(f"grafana dashboard '{_uid}' has query panels", False, "no expr found")
            continue
        check(f"dashboard '{_uid}' query panels present", True, f"panels_with_expr={len(_exprs)}")

        for _title, _expr in _exprs:
            _q = f"{PROMETHEUS}/api/v1/query?query=" + urllib.parse.quote(_expr)
            _st, _bd = http_get(_q, timeout=10.0)
            _n = -1
            if _st == 200:
                try:
                    _n = len(json.loads(_bd).get("data", {}).get("result", []))
                except (json.JSONDecodeError, KeyError, TypeError):
                    _n = -1
            check(
                f"panel expr returns data [{_uid}] {_title}",
                _n >= 1,
                f"series={_n} expr={_expr[:90]}",
            )

    # ===== E2E-22: 抓取清单防漂移 =====
    # 背景: EXPECTED_TARGETS 是 prometheus.yml targets 的**手抄副本**。改 prometheus.yml
    #   而不同步常量，smoke 不会发现；更糟的是当初 llm-service 暴露 /metrics 却两边
    #   都没有 ⇒ 该缺口结构上不可能被本 smoke 检出（E2E-22 修复）。
    # 本断言直接从 prometheus.yml 解析 targets，与常量双向比对。
    _prom_yml_path = os.path.normpath(os.path.join(
        os.path.dirname(os.path.abspath(__file__)),
        "..", "deploy", "prometheus", "prometheus.yml",
    ))
    try:
        with open(_prom_yml_path, "r", encoding="utf-8") as fh:
            _prom_txt = fh.read()
        _in_targets = False
        _yml_targets = set()
        for _line in _prom_txt.splitlines():
            # 形式 A: `- targets: ["host:port", ...]`（单行内联，apisix/sw-oap 用此形式）
            _m_inline = re.match(r'^\s*-\s*targets:\s*\[(.*)\]\s*$', _line)
            if _m_inline:
                for _t in re.findall(r'"([^"]+)"', _m_inline.group(1)):
                    _yml_targets.add(_t)
                _in_targets = False
                continue
            # 形式 B: `- targets:` 后跟缩进列表项
            if re.match(r"^\s*-\s*targets:\s*$", _line):
                _in_targets = True
                continue
            if _in_targets:
                _m = re.match(r'^\s*-\s*"?([\w.\-]+:\d+)"?\s*$', _line)
                if _m:
                    _yml_targets.add(_m.group(1))
                elif _line.strip() and not _line.strip().startswith("#"):
                    _in_targets = False
        _expect_set = set(EXPECTED_TARGETS)
        _missing = _expect_set - _yml_targets
        _biz = {t for t in _yml_targets if "svc" in t or "llm-service" in t}
        _biz_uncovered = _biz - _expect_set
        check(
            "prometheus.yml targets superset of EXPECTED_TARGETS (清单无陈旧项)",
            not _missing,
            f"missing_in_yml={sorted(_missing)} yml_targets={sorted(_yml_targets)}",
        )
        check(
            "every business-svc target is covered by EXPECTED_TARGETS",
            not _biz_uncovered,
            f"uncovered={sorted(_biz_uncovered)} (抓到了但常量漏登记 ⇒ 漂移会再次发生)",
        )
    except OSError as e:
        check("prometheus.yml readable for target drift check", False, f"{type(e).__name__}: {e}")

    # ===== PR-OBS-8: observability compose runbook 文档 断言 =====

    # 断言 12: docs/deployment/runbook/observability-compose.md 存在 + 含 5 节标题
    runbook_path = os.path.join(
        os.path.dirname(os.path.abspath(__file__)),
        "..",
        "docs",
        "deployment",
        "runbook",
        "observability-compose.md",
    )
    runbook_path = os.path.normpath(runbook_path)
    expected_sections = [
        "启动后第一件事",
        "看 targets",
        "看 logs",
        "看 trace",
        "故障排查",
    ]
    if not os.path.exists(runbook_path):
        check(
            "runbook observability-compose.md exists + 5 sections",
            False,
            f"file missing: {runbook_path}",
        )
    else:
        try:
            with open(runbook_path, "r", encoding="utf-8") as f:
                content = f.read()
            missing_sections = [
                s for s in expected_sections if s not in content
            ]
            if missing_sections:
                check(
                    "runbook observability-compose.md contains all 5 sections",
                    False,
                    f"missing={missing_sections}, file_len={len(content)}",
                )
            else:
                check(
                    "runbook observability-compose.md exists + 5 sections",
                    True,
                    f"file_len={len(content)}, sections={expected_sections}",
                )
        except OSError as e:
            check(
                "runbook observability-compose.md exists + 5 sections",
                False,
                f"{type(e).__name__}: {e}",
            )

    # ===== PR-OBS-7: Kafka consumer lag 监控 (接 Kafka Sprint A §1.4) =====

    # 断言 8: kafka-exporter :9308/metrics 含 kafka_consumergroup_lag series
    status, body = http_get(f"{KAFKA_EXPORTER}/metrics")
    if status == 0:
        check("kafka-exporter :9308/metrics reachable", False, body)
    elif status != 200:
        check("kafka-exporter :9308/metrics returns 200", False, f"HTTP {status}: {body[:100]}")
    else:
        has_lag = "kafka_consumergroup_lag" in body
        check(
            "kafka-exporter exposes kafka_consumergroup_lag series",
            has_lag,
            f"body_len={len(body)}, has_lag={has_lag}",
        )

    # 断言 9: prometheus scrape target 'kafka-exporter' UP
    # 与 PR-OBS-4 scrape targets 共享 prometheus /api/v1/targets
    status, body = http_get(f"{PROMETHEUS}/api/v1/targets?state=active")
    if status == 200:
        try:
            data = json.loads(body)
            active = data.get("data", {}).get("activeTargets", [])
            up_jobs = {
                t["labels"].get("job", "")
                for t in active
                if t.get("health") == "up"
            }
            check(
                "prometheus scrape target 'kafka-exporter' UP",
                "kafka-exporter" in up_jobs,
                f"up_jobs={sorted(up_jobs)}",
            )
        except (json.JSONDecodeError, KeyError, TypeError):
            pass  # 上面断言 1 已检过 JSON parse,跳过

    # 断言 10: grafana dashboard 'kafka-consumer-lag' provisioned
    status, body = http_get(
        f"{GRAFANA}/api/dashboards/uid/kafka-consumer-lag",
        headers={"Authorization": f"Basic {grafana_auth}"},
    )
    if status == 0:
        check("grafana dashboard 'kafka-consumer-lag' API reachable", False, body)
    elif status != 200:
        check(
            "grafana dashboard 'kafka-consumer-lag' returns 200",
            False,
            f"HTTP {status}: {body[:100]}",
        )
    else:
        try:
            data = json.loads(body)
            panels = data.get("dashboard", {}).get("panels", [])
            check(
                "grafana dashboard 'kafka-consumer-lag' provisioned (>=3 panels)",
                len(panels) >= 3,
                f"title={data.get('dashboard', {}).get('title', '?')!r}, panels={len(panels)}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("grafana kafka-consumer-lag JSON parseable", False, f"{type(e).__name__}: {e}")

    # 断言 10b (Stage 74): prometheus datasource 显式 uid
    # kafka-consumer-lag.json 3 个 panel 硬编码 datasource uid="prometheus";
    # datasource.yaml 若不写 uid,grafana 生成随机 uid → 面板报 datasource not found
    status, body = http_get(
        f"{GRAFANA}/api/datasources/name/Prometheus",
        headers={"Authorization": f"Basic {grafana_auth}"},
    )
    if status == 0:
        check("grafana datasource Prometheus name-API reachable", False, body)
    elif status != 200:
        check(
            "grafana datasource Prometheus name-API returns 200",
            False,
            f"HTTP {status}: {body[:100]}",
        )
    else:
        try:
            data = json.loads(body)
            check(
                "grafana prometheus datasource uid == 'prometheus'",
                data.get("uid") == "prometheus",
                f"uid={data.get('uid')!r}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("grafana datasource name-API JSON parseable", False, f"{type(e).__name__}: {e}")

    # 断言 11: prometheus alert rule 文件 KafkaConsumerGroupLagHigh 存在
    # prometheus rules path 是 container 内路径,我们用 API 查询 rule_files + alerting rules
    # Stage 86: 同一解析顺带断言 OutboxEventsDead (outbox-dead.yml) 已加载
    status, body = http_get(f"{PROMETHEUS}/api/v1/rules")
    loaded_alerts: set[str] = set()
    if status == 0:
        check("prometheus /api/v1/rules reachable", False, body)
    elif status != 200:
        check("prometheus /api/v1/rules returns 200", False, f"HTTP {status}: {body[:100]}")
    else:
        try:
            data = json.loads(body)
            rule_groups = data.get("data", {}).get("groups", [])
            for group in rule_groups:
                for rule in group.get("rules", []):
                    if rule.get("type") == "alerting":
                        loaded_alerts.add(rule.get("name", ""))
            check(
                "prometheus alert rule 'KafkaConsumerGroupLagHigh' loaded",
                "KafkaConsumerGroupLagHigh" in loaded_alerts,
                f"groups={len(rule_groups)}, total_rules={sum(len(g.get('rules', [])) for g in rule_groups)}",
            )
            check(
                "prometheus alert rule 'OutboxEventsDead' loaded (Stage 86)",
                "OutboxEventsDead" in loaded_alerts,
                f"loaded_alerts={sorted(loaded_alerts)}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("prometheus rules JSON parseable", False, f"{type(e).__name__}: {e}")

    # ===== Stage 86 (kafka-reliability-gaps.md §3.6): dead 告警接 alertmanager =====

    # 断言 13: alertmanager /-/healthy 200
    status, body = http_get(f"{ALERTMANAGER}/-/healthy")
    if status == 0:
        check("alertmanager /-/healthy reachable", False, body)
    else:
        check(
            "alertmanager /-/healthy returns 200",
            status == 200,
            f"HTTP {status}: {body[:80]}",
        )

    # 断言 14: prometheus 已发现 alertmanager (alerting 段接线生效)
    # 注: prometheus v2.51 该端点返回 data.activeAlertmanagers[].url（非 labels.instance）
    status, body = http_get(f"{PROMETHEUS}/api/v1/alertmanagers")
    if status == 200:
        try:
            data = json.loads(body)
            am_urls = [
                a.get("url", "")
                for a in data.get("data", {}).get("activeAlertmanagers", [])
            ]
            check(
                "prometheus discovered alertmanager target",
                any("alertmanager" in u and "9093" in u for u in am_urls),
                f"activeAlertmanagers={am_urls}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("prometheus alertmanagers JSON parseable", False, f"{type(e).__name__}: {e}")
    else:
        check("prometheus /api/v1/alertmanagers returns 200", False, f"HTTP {status}")

    # 断言 15: dead 计数器 series 可从 prometheus 查到 (端到端: chat-svc → scrape → TSDB)
    # counter 初始值为 0 也暴露 series,所以此断言不依赖真实 dead 行
    query_url = (
        f"{PROMETHEUS}/api/v1/query?query="
        + urllib.parse.quote("emotion_echo_outbox_events_dead_total")
    )
    status, body = http_get(query_url, timeout=10.0)
    if status == 200:
        try:
            data = json.loads(body)
            series = data.get("data", {}).get("result", [])
            check(
                "prometheus exposes emotion_echo_outbox_events_dead_total series",
                len(series) >= 1,
                f"series={len(series)}, values={[s.get('value', [None, '?'])[1] for s in series]}",
            )
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check("prometheus dead counter query JSON parseable", False, f"{type(e).__name__}: {e}")
    else:
        # series 缺失常见于 chat-svc 未起 / obs profile 未开——给出明确线索而非静默跳过
        check(
            "prometheus query emotion_echo_outbox_events_dead_total returns 200",
            False,
            f"HTTP {status}: {body[:80]} (chat-svc 未启动或 obs profile 未开?)",
        )

    # ===== PR-OBS-5: Loki + Promtail 断言 =====

    # 断言 4: loki ready
    status, body = http_get(f"{LOKI}/ready")
    if status == 0:
        check("loki /ready endpoint reachable", False, body)
    elif status != 200:
        check("loki /ready returns 200", False, f"HTTP {status}: {body[:100]}")
    else:
        check("loki /ready returns 200", True, body.strip()[:50])

    # 断言 5: loki query 返非空 —— **必须非空，不再对空结果放行**
    #
    # E2E-21 修订：原先这里是 "result 为空也判 PASS"（SKIP 文案），
    # 等于**一个完全没有日志的 Loki 也能全绿**——恰恰是 E2E-F-07
    # （promtail 的 services job 指向谁也不写的 /var/log/services/*.log，
    # 一条业务日志都采不到）长期没被发现的原因。
    # 现在空结果一律 FAIL。
    # 先自造流量，再断言。
    #
    # 为什么必须造：Loki 的 instant query 默认只看最近 1 小时，promtail→Loki 还有
    # ~10s 推送延迟。2026-09-28 实测踩过：一段时间没跑带 file-logger 的请求，
    # {job="apisix"} 恒 0 —— 断言失败但日志链路本身是好的。
    # 这类"断言依赖时序"的假红必须从脚本里消除，而不是靠人记得先点一下页面。
    #
    # 走哪条路由很关键：health 路由（200-205）**刻意不挂 file-logger**
    # （健康检查高频，落 access log 会把有用日志冲掉，见 seed.sh HEALTH_PLUGINS），
    # 所以必须打 route 100（/api/v1/*）和 route 110（登录白名单）才造得出日志。
    def _seed_loki_traffic() -> None:
        import urllib.request as _r

        def _hit(method: str, url: str, body: bytes | None = None) -> None:
            try:
                req = _r.Request(url, data=body, method=method)
                if body is not None:
                    req.add_header("Content-Type", "application/json")
                _r.urlopen(req, timeout=5).read()
            except Exception:
                pass  # 4xx/5xx 同样会产出 access log，这里只关心"请求打到了网关"

        gateway = os.environ.get("APISIX_GATEWAY", "http://localhost:19080")
        _hit("GET", f"{gateway}/api/v1/users/me")                       # route 100
        _hit("POST", f"{gateway}/api/v1/auth/login", b'{"username":"__smoke__","password":"__smoke__"}')  # route 110
        time.sleep(12)  # 等 promtail 读文件 + 推送到 loki

    _seed_loki_traffic()

    def _loki_count(label, query, expect_min=1, fresh_seconds=None):
        """查 Loki 并断言结果非空；返回 (stream 数, log 行数)。

        fresh_seconds: 只看最近 N 秒的数据。**必须传**，否则是弱断言 ——
        Loki instant query 默认回看 1 小时，采集早就断了也照样能查到存量日志。
        实测（2026-09-28）：把 promtail 的采集过滤器改坏（采不到任何容器），
        用默认 1 小时窗口查 {job="services"} 仍然全绿；加上新鲜度窗口后立刻变红。
        """
        # ⚠️ Loki ≥ 3.0 **移除了日志查询的 instant 端点**：对日志选择器调
        # /loki/api/v1/query 会返 400 "log queries are not supported as an
        # instant query type, please change your query to a range query type"。
        # 必须用 /query_range。（E2E-F-147 升级 2.9.4 → 3.2.0 时实测撞到。）
        import datetime as _dt
        end = _dt.datetime.now(_dt.timezone.utc)
        start = end - _dt.timedelta(seconds=(fresh_seconds if fresh_seconds is not None else 3600))
        url = (f"{LOKI}/loki/api/v1/query_range?query=" + urllib.parse.quote(query)
               + "&start=" + str(int(start.timestamp() * 1e9))
               + "&end=" + str(int(end.timestamp() * 1e9))
               + "&limit=100&direction=backward")
        st, bd = http_get(url, timeout=10.0)
        if st == 0:
            check(f"loki query reachable [{label}]", False, bd)
            return 0, 0
        if st != 200:
            check(f"loki query returns 200 [{label}]", False, f"HTTP {st}: {bd[:100]}")
            return 0, 0
        try:
            data = json.loads(bd)
            loki_results = data.get("data", {}).get("result", [])
        except (json.JSONDecodeError, KeyError, TypeError) as e:
            check(f"loki query JSON parseable [{label}]", False, f"{type(e).__name__}: {e}")
            return 0, 0
        if not isinstance(loki_results, list):
            check(f"loki query JSON valid [{label}]", False, f"unexpected type: {type(loki_results)}")
            return 0, 0
        lines = sum(len(r.get("values", [])) for r in loki_results)
        check(
            f"loki query [{label}] returns results (非空，空即 FAIL)",
            len(loki_results) >= expect_min and lines >= expect_min,
            f"streams={len(loki_results)}, log_lines={lines}, query={query}",
        )
        return len(loki_results), lines

    _loki_count("job=apisix", '{job="apisix"}', fresh_seconds=FRESH_WINDOW)

    # 断言 5b: 业务服务日志真的进了 Loki（E2E-F-07 的核心断言）
    # promtail 走 docker_sd_configs 采容器 stdout，svc label 由容器名推导。
    # 期望至少 1 个业务 svc 有日志 —— 注意不是 6 个：并非所有服务都在 smoke
    # 运行窗口内产生日志（未起/无流量）。要确认"每个服务都通"见 E2E-21 报告。
    svc_streams, svc_lines = _loki_count("job=services", '{job="services"}', fresh_seconds=FRESH_WINDOW)

    # 断言 5c: 采到的是结构化 JSON 且带 svc 字段（不是纯文本行）
    if svc_streams > 0:
        import datetime as _dt
        _end = _dt.datetime.now(_dt.timezone.utc)
        _start = _end - _dt.timedelta(seconds=FRESH_WINDOW)
        url = (f"{LOKI}/loki/api/v1/query_range?query=" + urllib.parse.quote('{job="services"}')
               + "&start=" + str(int(_start.timestamp() * 1e9))
               + "&end=" + str(int(_end.timestamp() * 1e9)) + "&limit=100")
        st, bd = http_get(url, timeout=10.0)
        svc_field_ok = False
        sample = ""
        try:
            loki_results = json.loads(bd).get("data", {}).get("result", [])
            for r in loki_results:
                for _ts, line in r.get("values", [])[:20]:
                    try:
                        obj = json.loads(line)
                    except (json.JSONDecodeError, TypeError):
                        continue
                    if obj.get("svc") and obj.get("level"):
                        svc_field_ok = True
                        sample = line[:160]
                        break
                if svc_field_ok:
                    break
        except (json.JSONDecodeError, KeyError, TypeError):
            pass
        check(
            "loki 业务日志是结构化 JSON（含 svc/level 字段）",
            svc_field_ok,
            sample or "未找到可解析的 JSON 日志行（纯文本即 FAIL）",
        )
    else:
        check(
            "loki 业务日志是结构化 JSON（含 svc/level 字段）",
            False,
            "无业务日志可解析（断言 5b 已 FAIL，此处连带 FAIL）",
        )

    # 断言 5d: 采集没有把无关容器 / Loki 自身日志卷进来（回环 + 噪声）
    #
    # ⚠️ 不要用 /label/container/values 判这条：Loki 的 label 索引**有长记忆**
    # （实测把 filters 去掉污染了索引后，即便立刻恢复配置，
    #   该端点仍持续返回 postgres/kafka/loki/promtail → 假红）。
    # 正确口径是查"新鲜窗口内实际入库的 stream 的 container 标签"。
    import datetime as _dt5
    _end5 = _dt5.datetime.now(_dt5.timezone.utc)
    _start5 = _end5 - _dt5.timedelta(seconds=FRESH_WINDOW)
    url = (f"{LOKI}/loki/api/v1/query_range?query=" + urllib.parse.quote('{job="services"}')
           + "&start=" + str(int(_start5.timestamp() * 1e9))
           + "&end=" + str(int(_end5.timestamp() * 1e9)) + "&limit=100")
    st, bd = http_get(url, timeout=10.0)
    if st == 200:
        try:
            vals = sorted({
                r.get("stream", {}).get("container")
                for r in json.loads(bd).get("data", {}).get("result", [])
                if r.get("stream", {}).get("container")
            })
        except (json.JSONDecodeError, TypeError, AttributeError):
            vals = []
        noisy = [
            v for v in vals
            if v in ("emotion-echo-loki", "emotion-echo-promtail",
                     "emotion-echo-postgres", "emotion-echo-kafka", "emotion-echo-redis")
        ]
        check(
            "promtail 未采到 loki/promtail/中间件容器（日志回环 + 噪声）",
            not noisy,
            f"新鲜窗口内出现无关容器: {noisy}" if noisy
            else f"新鲜窗口 container 标签集: {vals}",
        )
    else:
        check("loki label/container/values reachable", False, f"HTTP {st}")

    # 断言 6: APISIX access.log 落盘文件非空 (volume mount 生效)
    # 路径: /tmp/apisix-access.log 在 APISIX 容器内,
    # 挂到 ./tmp/apisix-access.log 在 host
    # 注意: 容器内文件需通过 docker exec 检查
    # 容器状态检查先于文件检查 — apisix Restarting 时 docker exec 会失败
    try:
        import subprocess
        # 先查容器状态
        inspect = subprocess.run(
            ["docker", "inspect", "emotion-echo-apisix", "--format", "{{.State.Status}}"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        container_status = inspect.stdout.strip()
        if container_status != "running":
            check(
                "apisix /tmp/apisix-access.log exists and non-empty",
                False,
                f"container status={container_status!r} (需 running 才能 docker exec;Nacos 注册 500 阻塞常见)",
            )
        else:
            result = subprocess.run(
                ["docker", "exec", "emotion-echo-apisix", "test", "-s", "/tmp/apisix-access.log"],
                capture_output=True,
                text=True,
                timeout=5,
            )
            # test -s: 文件存在且 size > 0
            check(
                "apisix /tmp/apisix-access.log exists and non-empty",
                result.returncode == 0,
                f"exit={result.returncode}, stderr={result.stderr[:100]}",
            )
    except (subprocess.TimeoutExpired, FileNotFoundError) as e:
        check(
            "apisix /tmp/apisix-access.log check runs",
            False,
            f"{type(e).__name__}: {e}",
        )

    print()
    failed = [n for n, ok, _ in results if not ok]
    if failed:
        print(f"FAIL: {len(failed)} check(s) failed: {failed}")
        return 1
    print(f"PASS: {len(results)} check(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
