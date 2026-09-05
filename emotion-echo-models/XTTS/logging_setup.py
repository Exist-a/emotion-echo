"""Structured JSON logger for Python services (Stage 20-2 pattern)."""
import logging
import os
import sys


def setup_logging(name: str = __name__) -> logging.Logger:
    fmt = os.environ.get("LOG_FORMAT", "json").lower()
    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)

    handler = logging.StreamHandler(sys.stdout)
    if fmt == "json":
        try:
            from pythonjsonlogger import jsonlogger

            handler.setFormatter(
                jsonlogger.JsonFormatter(
                    "%(asctime)s %(levelname)s %(name)s %(message)s"
                )
            )
        except ImportError:
            handler.setFormatter(
                logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s")
            )
    else:
        handler.setFormatter(
            logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s")
        )

    root = logging.getLogger()
    root.handlers = [handler]
    root.setLevel(level)

    return logging.getLogger(name)
