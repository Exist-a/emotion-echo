"""
emotion-llm-service · 结构化日志配置（Stage 20-2）

提供 setup_logging() 函数，根据 LOG_FORMAT 环境变量决定：
  - json (默认，容器环境推荐)：每行一个 JSON 对象，方便 log aggregator 解析
  - text (开发环境)：纯文本格式，保留传统可读性

字段说明（JSON 模式）—— **E2E-F-148 与 Go 侧 shared/pkg/logging 对齐**：
  - time:    ISO8601 UTC 时间戳（带毫秒，**与 Go 同名**）
  - level:   日志级别（INFO/WARN/ERROR/DEBUG，与 Go 同名）
  - msg:     消息文本（与 Go 同名）
  - svc:     服务名（与 Go 同名，缺省 "llm-service"）
  - exc:     异常堆栈（如有）
  - 其它：  通过 logger.info("...", extra={...}) 传入的字段

兼容保留（勿删，仓内解析脚本/看板可能仍在用）：
  - ts:      = time 的同值别名
  - logger:  logger 名称（通常是 __name__）

为什么要 time + svc：Go 侧每行都带 time/level/msg/svc/trace_id，字段名一致时
一条 LogQL 就能同时查 6 个 Go 服务与 llm-service；名字不一致就得写两套查询，
按时间过滤都会踩空。

使用：
  from logging_setup import setup_logging
  setup_logging()
  logger = logging.getLogger(__name__)
  logger.info("analyze done", extra={"message_id": 42, "emotion": "happy"})
"""
import json
import logging
import os
import sys
import time

# 缺省服务名（Go 侧对应 shared/pkg/logging.SetGlobalSvc 的取值）
DEFAULT_SVC = os.environ.get("SVC_NAME", "llm-service")

# 一些 record 的内置字段，序列化时跳过
_RESERVED = {
    "name", "msg", "args", "levelname", "levelno", "pathname", "filename",
    "module", "exc_info", "exc_text", "stack_info", "lineno", "funcName",
    "created", "msecs", "relativeCreated", "thread", "threadName",
    "processName", "process", "message", "asctime", "taskName",
}


class JsonFormatter(logging.Formatter):
    """JSON formatter：一行一个 JSON 对象到 stdout。"""

    def format(self, record: logging.LogRecord) -> str:
        # ISO8601 UTC 时间戳（带毫秒）
        ts = time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(record.created))
        ts = f"{ts}.{int(record.msecs):03d}Z"

        # E2E-F-148：time/level/msg/svc 与 Go 侧同名字段优先（一条 LogQL 通吃）；
        # ts/logger 保留为兼容别名（历史解析脚本/看板可能依赖）。
        log_obj = {
            "time": ts,
            "level": record.levelname,
            "msg": record.getMessage(),
            "svc": record.__dict__.get("svc") or DEFAULT_SVC,
            # —— 以下为兼容别名
            "ts": ts,
            "logger": record.name,
        }
        if record.exc_info:
            log_obj["exc"] = self.formatException(record.exc_info)

        # 把 extra= 传入的字段并入顶层（svc/trace_id 等已在上方显式处理，extra 传入时覆盖之）
        for k, v in record.__dict__.items():
            if k not in _RESERVED and not k.startswith("_"):
                try:
                    json.dumps(v)
                    log_obj[k] = v
                except (TypeError, ValueError):
                    log_obj[k] = repr(v)

        return json.dumps(log_obj, ensure_ascii=False)


class TextFormatter(logging.Formatter):
    """传统文本格式：'[时间] [级别] [logger] msg'"""

    def __init__(self):
        super().__init__(fmt="%(asctime)s [%(levelname)s] [%(name)s] %(message)s",
                         datefmt="%Y-%m-%d %H:%M:%S")


def setup_logging(level: str = None) -> None:
    """配置 root logger。

    读取环境变量：
      LOG_FORMAT = json (default) | text
      LOG_LEVEL  = INFO (default) | DEBUG | WARNING | ERROR
    """
    fmt = (level or os.environ.get("LOG_LEVEL", "INFO")).upper()
    log_format = os.environ.get("LOG_FORMAT", "json").lower()

    handler = logging.StreamHandler(sys.stdout)
    if log_format == "json":
        handler.setFormatter(JsonFormatter())
    else:
        handler.setFormatter(TextFormatter())

    root = logging.getLogger()
    # 清掉已有 handlers（避免重复输出）
    for h in list(root.handlers):
        root.removeHandler(h)
    root.addHandler(handler)
    root.setLevel(fmt)

    # 把 grpc 内部 logger 调成 INFO（太多 DEBUG 会刷屏）
    logging.getLogger("grpc").setLevel(logging.INFO)
