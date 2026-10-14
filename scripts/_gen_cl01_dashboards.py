#!/usr/bin/env python3
"""CL-01 Phase 7：生成 business / reliability / logs 三个看板 JSON。

一次性生成脚本（不入库为长期工具）——面板结构统一，手写三份 JSON 易漂移。
所有 expr 都按「零值也必须返回 series」写（`or vector(0)`），因为
scripts/smoke_observability.py 的 _panel_exprs 断言每个面板 expr 非空：
否则「没数据」与「面板坏了」在 UI 上无法区分（E2E-22 实测坑）。
"""
import json
import os

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "deploy", "grafana", "dashboards")


def ts(title, expr, x, y, w=12, h=8, unit="short", desc="", legend="{{instance}}"):
    return {
        "type": "timeseries",
        "title": title,
        "id": None,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "datasource": {"type": "prometheus", "uid": "prometheus"},
        "targets": [{"expr": expr, "refId": "A", "legendFormat": legend}],
        "fieldConfig": {"defaults": {"unit": unit, "custom": {"lineWidth": 2, "fillOpacity": 10}}, "overrides": []},
        "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "multi"}},
        "description": desc,
    }


def stat(title, expr, x, y, w=6, h=5, unit="short", desc="", thresholds=None):
    steps = [{"color": "green", "value": None}]
    for v, c in (thresholds or []):
        steps.append({"color": c, "value": v})
    return {
        "type": "stat",
        "title": title,
        "id": None,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "datasource": {"type": "prometheus", "uid": "prometheus"},
        "targets": [{"expr": expr, "refId": "A", "instant": True}],
        "fieldConfig": {"defaults": {"unit": unit, "thresholds": {"mode": "absolute", "steps": steps}}, "overrides": []},
        "options": {
            "colorMode": "background",
            "graphMode": "none",
            "textMode": "auto",
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
        },
        "description": desc,
    }


def table(title, expr, x, y, w=12, h=10, desc="", legend="{{instance}}"):
    return {
        "type": "table",
        "title": title,
        "id": None,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "datasource": {"type": "prometheus", "uid": "prometheus"},
        "targets": [{"expr": expr, "refId": "A", "instant": True, "format": "table", "legendFormat": legend}],
        "fieldConfig": {"defaults": {"custom": {"align": "auto", "cellOptions": {"type": "auto"}, "filterable": True}}, "overrides": []},
        "options": {"showHeader": True},
        "description": desc,
    }


def logs(title, expr, x, y, w=24, h=10, desc=""):
    return {
        "type": "logs",
        "title": title,
        "id": None,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "datasource": {"type": "loki", "uid": "loki"},
        "targets": [{"expr": expr, "refId": "A", "datasource": {"type": "loki", "uid": "loki"}}],
        "options": {"showTime": True, "wrapLogMessage": True, "sortOrder": "Descending"},
        "description": desc,
    }


def dash(uid, title, tags, panels, desc):
    for i, p in enumerate(panels, start=1):
        p["id"] = i
    return {
        "uid": uid,
        "title": title,
        "tags": tags,
        "timezone": "browser",
        "schemaVersion": 39,
        "version": 1,
        "refresh": "30s",
        "editable": True,
        "graphTooltip": 1,
        "time": {"from": "now-6h", "to": "now"},
        "description": desc,
        "panels": panels,
        "annotations": {"list": []},
        "links": [],
    }


# ============================================================
# 1) business —— 融合/业务面（回答 T-11："是否在降级 / 是否在跟 mock 对话"）
# ============================================================
business = dash(
    "emotion-echo-business",
    "Emotion-Echo · 业务面（融合链路）",
    ["cl-01", "business", "fusion"],
    [
        stat("融合调用速率（5m）", "sum(increase(emotion_fusion_calls_total[5m])) or vector(0)", 0, 0, 6, 5,
             desc="fusion 整体调用次数（T-06 接线的 emotion_fusion_calls_total）。\n为 0 = 融合链路根本没跑（worker candidates=0）。"),
        stat("融合失败（5m）", "sum(increase(emotion_fusion_calls_total{result=\"failure\"}[5m])) or vector(0)", 6, 0, 6, 5,
             thresholds=[(1, "red")], desc="失败会让 /reports/* 的情绪分布退化为只有文本。"),
        stat("降级次数（5m）", "sum(increase(emotion_echo_fusion_fallback_total[5m])) or vector(0)", 12, 0, 6, 5,
             thresholds=[(1, "orange")],
             desc="**T-11 核心**：>0 说明融合正在走回退（llm_to_late / late_to_skip）。\ndev 环境 LLM_BASE_URL 为空 ⇒ 这是预期态，不是故障。"),
        stat("Worker tick 速率（5m）", "sum(rate(emotion_echo_fusion_worker_tick_total{outcome=\"ok\"}[5m])) or vector(0)", 18, 0, 6, 5,
             unit="ops", desc="FusionWorker 每 5s 一次 tick ⇒ 正常约 0.2/s。为 0 = 循环停摆。"),
        ts("融合调用（按模态 × 结果）", "sum by (modality, result) (rate(emotion_fusion_calls_total[5m])) or vector(0)",
           0, 5, 12, 8, legend="{{modality}} / {{result}}",
           desc="modality ∈ text|face|voice|fused（低基数，由 fusionModalityLabel 归一）。"),
        ts("融合回退（按 stage）", "sum by (stage) (rate(emotion_echo_fusion_fallback_total[5m])) or vector(0)",
           12, 5, 12, 8, legend="{{stage}}",
           desc="llm_to_late = LLM fuser 不可用→加权 late fusion 兜底；late_to_skip = 连兜底都不可用→跳过融合。"),
        ts("LLM 融合调用（按 outcome）", "sum by (outcome) (rate(emotion_echo_fusion_llm_call_total[5m])) or vector(0)",
           0, 13, 12, 8, legend="{{outcome}}",
           desc="**回答\"是否在跟 mock/降级对话\"**：outcome 里出现 timeout / circuit_open / http_5xx\n即 LLM 上游有问题；只有 success 才是真 LLM 融合。\n（无 series = 从未调用过 LLM fuser，如 dev LLM_BASE_URL 为空。）"),
        ts("LLM 融合 p95 延迟", "histogram_quantile(0.95, sum by (le, outcome) (rate(emotion_echo_fusion_llm_latency_seconds_bucket[5m]))) or vector(0)",
           12, 13, 12, 8, unit="s", legend="{{outcome}}"),
        ts("融合耗时 p95（按模态）", "histogram_quantile(0.95, sum by (le, modality) (rate(emotion_fusion_duration_seconds_bucket[5m]))) or vector(0)",
           0, 21, 12, 8, unit="s", legend="{{modality}}"),
        ts("物化视图刷新", "sum(rate(emotion_echo_mv_refresh_success_total[5m])) or vector(0)", 12, 21, 6, 8,
           legend="success", desc="报表读的 *_v 视图由定时刷新维护；成功/失败应同时看。"),
        ts("物化视图刷新失败", "sum(rate(emotion_echo_mv_refresh_fail_total[5m])) or vector(0)", 18, 21, 6, 8,
           legend="fail", desc=">0 = 报表数据可能陈旧。"),
    ],
    "CL-01 Phase 7 · 业务面看板。回答两个问题：① 融合链路是否在跑；② 是否处于降级/mock 态（T-11）。\n"
    "指标口径见 deploy/prometheus/rules/cl01-health-business.yml 顶部注释。",
)

# ============================================================
# 2) reliability —— 可靠性面（DLQ / outbox / 就绪 / 5xx）
# ============================================================
reliability = dash(
    "emotion-echo-reliability",
    "Emotion-Echo · 可靠性面（DLQ / Outbox / 就绪）",
    ["cl-01", "reliability"],
    [
        stat("Outbox 死信行", "sum(emotion_echo_outbox_events_dead_total) or vector(0)", 0, 0, 6, 5,
             thresholds=[(1, "red")], desc="chat-svc outbox 重试耗尽的行数。>0 = 有事件永远发不出去。"),
        stat("Outbox 走内存回退", "sum(emotion_echo_outbox_sent_via_fallback_total) or vector(0)", 6, 0, 6, 5,
             thresholds=[(1, "orange")],
             desc="**重要**：>0 表示 Kafka producer 初始化失败、outbox 静默改投内存 ⇒\n"
                  "**后续事件不会进 Kafka，下游 consumer 全部收不到**（2026-10-14 实测该值为 1）。"),
        stat("DLQ 投递失败（5m）", "sum(increase(emotion_echo_dlq_publish_total{result=\"failure\"}[5m])) + sum(increase(emotion_echo_analytics_dlq_publish_total{result=\"failure\"}[5m])) or vector(0)",
             12, 0, 6, 5, thresholds=[(1, "red")],
             desc="业务消息已 MarkMessage 前进、DLQ 也没收到 = 事件彻底丢失（双失败黑洞）。"),
        stat("就绪探针失败（5m）", "sum(increase(emotion_echo_http_requests_total{path=\"/health/ready\",status!~\"2..\"}[5m])) or vector(0)",
             18, 0, 6, 5, thresholds=[(1, "red")],
             desc="**T-15 / D-05**：/health/ready 返非 2xx 的次数。readiness=依赖可用性，\n503 表示实例实际不可用（进程还在、up 仍为 1）。"),
        ts("HTTP 5xx 速率（按服务）", "sum by (service) (rate(emotion_echo_http_requests_total{status=~\"5..\"}[5m])) or vector(0)",
           0, 5, 12, 8, legend="{{service}}"),
        ts("HTTP 4xx 速率（按服务）", "sum by (service) (rate(emotion_echo_http_requests_total{status=~\"4..\"}[5m])) or vector(0)",
           12, 5, 12, 8, legend="{{service}}"),
        ts("DLQ 投递（按结果）", "sum by (result) (rate(emotion_echo_dlq_publish_total[5m])) or vector(0)",
           0, 13, 12, 8, legend="ai-svc {{result}}"),
        ts("DLQ 投递（analytics，按结果）", "sum by (result) (rate(emotion_echo_analytics_dlq_publish_total[5m])) or vector(0)",
           12, 13, 12, 8, legend="analytics-svc {{result}}"),
        table("Tracing 初始化失败（按服务）", "sum by (service) (emotion_echo_skywalking_init_failed_total) or vector(0)", 0, 21, 12, 8,
              desc="**>0 = 该服务当前完全没有链路追踪**（tracer 初始化失败后不再重试，\n"
                   "SkyWalking UI 的服务列表里会缺这个服务）。2026-10-14 实测 chat-svc / assessment-svc = 1，\n"
                   "重启两容器后归零（OAP 当时未就绪 ⇒ 启动竞态）。"),
        table("就绪探针通断（按服务 × 状态码）", "sum by (service, status) (increase(emotion_echo_http_requests_total{path=\"/health/ready\"}[1h])) or vector(0)",
              12, 21, 12, 8, legend="{{service}}",
              desc="健康检查每 30s 一次 ⇒ 1h 约 120 次 200。\n出现 503 行 = 该服务曾因依赖不可用被判 not ready。"),
    ],
    "CL-01 Phase 7 · 可靠性面看板。覆盖 DLQ 黑洞、outbox 静默回退、就绪探针、5xx 速率。\n"
    "注意 `outbox_sent_via_fallback_total > 0` 是**下游全链路静默失效**的早期信号。",
)

# ============================================================
# 3) logs —— 日志面（Loki；trace_id 关联，T-14）
# ============================================================
logs_dash = dash(
    "emotion-echo-logs",
    "Emotion-Echo · 日志面（Loki / trace_id 关联）",
    ["cl-01", "logs", "loki"],
    [
        logs("全部服务日志（{job=\"services\"}）", '{job="services"}', 0, 0, 24, 10,
             desc="7 个 svc + llm-service 的容器日志（promtail docker_sd 采集）。\n"
                  "每条 JSON 日志带 trace_id ⇒ 可与 SkyWalking 的 trace 对照。"),
        logs("ERROR 日志", '{job="services"} | detected_level="error"', 0, 10, 24, 10,
             desc="Loki 3.x 的 detected_level 由 promtail 从 JSON 的 level 字段推断（实测可用，非推断）。"),
        logs("APISIX 网关访问日志", '{job="apisix"}', 0, 20, 24, 10,
             desc="网关是唯一业务入口 ⇒ 这里是\"用户到底打了哪个端点\"的第一现场。"),
        logs("跨服务 trace 关联（改 trace_id 过滤）", '{job="services"} |= "trace_id"', 0, 30, 24, 10,
             desc="**T-14**：把 `|= \"trace_id\"` 换成具体 trace id（如 SkyWalking UI 里看到的），\n"
                  "即可一次拉出该请求在**多个服务**里的日志（2026-10-14 实测命中 BFF + ai-svc 两条流）。"),
    ],
    "CL-01 Phase 7 · 日志面看板（Loki datasource uid=loki）。\n"
    "smoke_observability.py 只对 prometheus 数据源的面板做 expr 非空断言，Loki 面板跳过（LogQL 不能喂 Prometheus）。",
)

for d in (business, reliability, logs_dash):
    path = os.path.normpath(os.path.join(OUT, d["uid"] + ".json"))
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(d, fh, ensure_ascii=False, indent=1)
        fh.write("\n")
    print("wrote", path, "panels=", len(d["panels"]))
