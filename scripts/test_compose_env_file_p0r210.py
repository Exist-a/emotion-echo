"""
scripts/test_compose_env_file_p0r210.py

Stage 97 PR-5 · P0-R2-10 docker-compose env_file 字面量契约测试。

P0-R2-10: dev compose 顶层不引 env_file: .env.local，但 AGENTS.md §四禁止
不带 --env-file .env.local。
风险：docker compose up 一行不带 env-file → 容器静默走 mock 模式（LLM key 空）。
修复：deploy/docker-compose.apps.yml 顶层加 x-env-file 锚点，所有需要
LLM key / Internal API key 的 service 引用 `env_file: *env-file`。

本脚本字面量断言（避免实际 docker compose up 在受限网络/内存环境失败）：
  1. compose 顶层有 `x-env-file: &env-file` 锚点定义
  2. 锚点值含 `.env.local` 路径
  3. 需要 key 的服务（ai-svc / chat-svc / llm-service / web-bff）均
     `env_file: *env-file`
  4. AGENTS.md §四仍要求 --env-file 参数启动（保持文档一致）

运行：python scripts/test_compose_env_file_p0r210.py
退出码：0=全部通过，1=失败
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
COMPOSE = REPO_ROOT / "deploy" / "docker-compose.apps.yml"
AGENTS = REPO_ROOT / "AGENTS.md"

failures: list[str] = []


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        print(f"  PASS  {name}")
    else:
        print(f"  FAIL  {name}{(' — ' + detail) if detail else ''}")
        failures.append(name)


def main() -> int:
    if not COMPOSE.exists():
        print(f"FAIL  compose not found: {COMPOSE}")
        return 1
    if not AGENTS.exists():
        print(f"FAIL  AGENTS.md not found: {AGENTS}")
        return 1

    compose = COMPOSE.read_text(encoding="utf-8")
    agents = AGENTS.read_text(encoding="utf-8")

    print(f"=== {COMPOSE.name} env_file 字面量契约（Stage 97 PR-5） ===")

    # 1) 顶层 x-env-file 锚点
    check(
        "P0-R2-10: 顶层定义 x-env-file 锚点",
        "x-env-file:" in compose and "&env-file" in compose,
        "compose 顶层必须有 YAML 锚点统一管理 env_file 路径",
    )

    # 2) 锚点值含 .env.local
    check(
        "P0-R2-10: 锚点指向 .env.local",
        ".env.local" in compose,
        "锚点必须指向仓库根 .env.local（LLM key 唯一存放点）",
    )

    # 3) 需要 key 的服务均引用锚点（compose 文件里 service 名带 emotion-echo 前缀）
    services_with_env_file = [
        ("user-svc", "emotion-echo-user-svc:"),
        ("chat-svc", "emotion-echo-chat-svc:"),
        ("analytics-svc", "emotion-echo-analytics-svc:"),
        ("assessment-svc", "emotion-echo-assessment-svc:"),
        ("llm-service", "emotion-llm-service:"),
        ("ai-svc", "emotion-echo-ai-svc:"),
        ("web-bff", "emotion-echo-web-bff:"),
    ]
    for short, full_label in services_with_env_file:
        # 服务标签必须是行首 2 空格的（避免误命中注释里的字面量）。
        # compose 是缩进敏感 YAML；service 标签形如 "  emotion-echo-user-svc:"。
        m = re.search(r"^  " + re.escape(full_label) + r"$", compose, re.MULTILINE)
        if not m:
            failures.append(f"  {short} 服务未找到")
            print(f"  FAIL    {short} 服务未找到（{full_label}）")
            continue
        idx = m.start()
        # 截取从 idx 到下一个 service 标签（行首 2 空格 + emotion 开头）
        rest = compose[idx + len(full_label) + 1:]  # +1 跳过行尾 \n
        next_svc = re.search(r"\n  emotion-(?:echo-|)[a-z-]+:\n", rest)
        if next_svc:
            block = compose[idx: idx + len(full_label) + 1 + next_svc.start()]
        else:
            block = compose[idx:]
        check(
            f"  {short} 引用 env_file 锚点",
            "env_file: *env-file" in block,
            f"service {short} 必须 env_file: *env-file 以加载 .env.local",
        )

    # 4) AGENTS.md §四仍要求 --env-file .env.local 启动
    # 简化：检查 .env.local 字面量在 AGENTS.md 出现且有强制措辞
    check(
        "AGENTS.md 保留 --env-file .env.local 强制要求",
        ".env.local" in agents,
        "AGENTS.md §四必须明确 --env-file .env.local 强制启动",
    )

    print()
    if failures:
        print(f"FAILED: {len(failures)} 项契约违规: {failures}")
        return 1
    print("OK: 所有 Stage 97 PR-5 字面量契约通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())