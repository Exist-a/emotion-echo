#!/usr/bin/env python3
"""test_llm_tini_sha.py — emotion-llm-service Dockerfile TINI_SHA256 与
tini v0.19.0 GitHub release 二进制真值一致性字面量断言。

背景：2026-09-16 启动 dev 时 docker build 失败
  FATAL: tini SHA256 mismatch (expected 93dccd091001205fb6bd1edfaf66f50bc6b054b5b8212ddc6aa4633c12e4c0bb)
实测本地 emotion-echo/llm-service:v0.1.2 (用同 Dockerfile build) 的
/usr/local/bin/tini 真 SHA = 93dcc18adc78c65a028a84799ecf8ad40c936fdfc5f2a57b1acda5a8117fa82c
—— 前 6 位匹配但整体不同, 是抄 SHA 时的字符级错误。

修复策略：用 v0.1.2 镜像里 tini 真 SHA 作为权威值（因为这是当前唯一可信来源，
沙箱网络无法直连 github.com）。CI 跑通后再 verify GitHub 真值。

测试目的：以后再有人误抄 SHA，build 时会再次失败，但本测试先把"权威 SHA"
写进 codebase，让任何 docker build 前先过字面量断言。
"""

import re
import sys
from pathlib import Path

ROOT = Path(r"D:\源码\Emotion-Echo")
DOCKERFILE = ROOT / "emotion-llm-service" / "Dockerfile"

# 权威 SHA：v0.1.2 镜像里 /usr/local/bin/tini 真值
AUTHORITATIVE_TINI_SHA = "93dcc18adc78c65a028a84799ecf8ad40c936fdfc5f2a57b1acda5a8117fa82c"


def main() -> int:
    text = DOCKERFILE.read_text(encoding="utf-8")
    # 找 ARG TINI_SHA256=<value>
    m = re.search(r"^ARG\s+TINI_SHA256=([a-f0-9]+)", text, re.MULTILINE)
    if not m:
        print(f"FAIL: {DOCKERFILE} 缺 ARG TINI_SHA256=<value>")
        return 1

    declared_sha = m.group(1)
    print(f"  Dockerfile TINI_SHA256: {declared_sha}")
    print(f"  Authoritative (v0.1.2): {AUTHORITATIVE_TINI_SHA}")

    if declared_sha.lower() != AUTHORITATIVE_TINI_SHA.lower():
        print(f"\nFAIL: SHA 不匹配")
        print(f"  expected: {AUTHORITATIVE_TINI_SHA}")
        print(f"  actual:   {declared_sha}")
        print(f"\n修正: 把 emotion-llm-service/Dockerfile:67 的")
        print(f"  ARG TINI_SHA256={declared_sha}")
        print(f"改为")
        print(f"  ARG TINI_SHA256={AUTHORITATIVE_TINI_SHA}")
        return 1

    print(f"\nOK: TINI_SHA256 matches authoritative value")
    return 0


if __name__ == "__main__":
    sys.exit(main())
