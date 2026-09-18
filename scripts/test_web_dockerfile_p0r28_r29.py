"""
scripts/test_web_dockerfile_p0r28_r29.py

Stage 97 PR-4 · P0-R2-8 + P0-R2-9 web Dockerfile 字面量契约测试。

P0-R2-8: web Dockerfile prod 阶段无 `USER node` → 以 root 跑 Node.js。
风险：容器逃逸 → 攻击者可改写 .output 引入恶意代码，破坏供应链。
修复：运行阶段加 `USER node`（alpine 镜像内置 node 用户）。

P0-R2-9: 原 Dockerfile `rm -f package-lock.json` + `npm install`
→ 不可复现 + 供应链风险（每次 install 解析 latest）。
修复：保留 lockfile + 改 `npm ci --omit=optional`（Stage 74 linux-musl 兼容
通过 --omit=optional 解决，不再删除 lockfile）。

本脚本字面量断言（避免实际 docker build 在受限网络/内存环境失败）：
  P0-R2-8: Dockerfile prod 阶段含 `USER node`
  P0-R2-9: Dockerfile 不含 `rm -f package-lock.json`，含 `npm ci`
  Bonus: 含 HEALTHCHECK（Node 容器应有健康探针）

运行：python scripts/test_web_dockerfile_p0r28_r29.py
退出码：0=全部通过，1=失败
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
WEB_DOCKERFILE = REPO_ROOT / "emotion-echo-web" / "Dockerfile"

failures: list[str] = []


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        print(f"  PASS  {name}")
    else:
        print(f"  FAIL  {name}{(' — ' + detail) if detail else ''}")
        failures.append(name)


def main() -> int:
    if not WEB_DOCKERFILE.exists():
        print(f"FAIL  Dockerfile not found: {WEB_DOCKERFILE}")
        return 1
    src = WEB_DOCKERFILE.read_text(encoding="utf-8")

    print(f"=== {WEB_DOCKERFILE.name} 字面量契约（Stage 97 PR-4） ===")

    # 切分构建 / 运行阶段：第二个 FROM 之后是 prod 阶段
    from_matches = list(re.finditer(r"^FROM\s+\S+", src, re.MULTILINE))
    if len(from_matches) < 2:
        print(f"FAIL  Dockerfile 必须 ≥2 个 FROM（builder + runner），找到 {len(from_matches)} 个")
        return 1
    prod_start = from_matches[1].start()
    prod_section = src[prod_start:]

    # P0-R2-8: prod 阶段必须 USER node
    check(
        "P0-R2-8: prod 阶段含 USER node",
        "USER node" in prod_section,
        "容器以 root 跑 Node.js → 容器逃逸 + 供应链风险",
    )

    # P0-R2-9: 整文件不应再 `rm -f package-lock.json`
    check(
        "P0-R2-9: 不再删除 package-lock.json",
        "rm -f package-lock.json" not in src,
        "每次 install 解析最新版本 → 不可复现 + 供应链风险",
    )
    check(
        "P0-R2-9: 改用 npm ci 保留 lockfile",
        "npm ci" in src,
        "npm ci 锁版本保证可复现构建",
    )

    # Bonus P2-R2-16: HEALTHCHECK（Node 容器应探活）
    check(
        "P2-R2-16: 含 HEALTHCHECK",
        "HEALTHCHECK" in src,
        "compose depends_on / APISIX 无法判定 web 是否就绪",
    )

    print()
    if failures:
        print(f"FAILED: {len(failures)} 项契约违规: {failures}")
        return 1
    print("OK: 所有 Stage 97 PR-4 字面量契约通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())
