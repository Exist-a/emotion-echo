"""
emotion-llm-service · 结构化日志配置（Stage 20-2）

提供 setup_logging() 函数，根据 LOG_FORMAT 环境变量决定：
  - json (默认，容器环境推荐)：每行一个 JSON 对象，方便 log aggregator 解析
  - text (开发环境)：纯文本格式，保留传统可读性

字段说明（JSON 模式）：
  - ts:      ISO8601 UTC 时间戳（带毫秒）
  - level:   日志级别（INFO/WARN/ERROR/DEBUG）
  - logger:  logger 名称（通常是 __name__）
  - msg:     消息文本
  - exc:     异常堆栈（如有）
  - 其它：  通过 logger.info("...", extra={...}) 传入的字段

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

        log_obj = {
            "ts": ts,
            "level": record.levelname,
            "logger": record.name,
            "msg": record.getMessage(),
        }
        if record.exc_info:
            log_obj["exc"] = self.formatException(record.exc_info)

        # 把 extra= 传入的字段并入顶层
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
