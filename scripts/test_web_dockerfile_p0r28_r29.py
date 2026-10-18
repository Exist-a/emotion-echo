"""
scripts/test_web_dockerfile_p0r28_r29.py

Stage 97 PR-4 · P0-R2-8 + P0-R2-9 web Dockerfile 字面量契约测试。

P0-R2-8: web Dockerfile prod 阶段无 `USER node` → 以 root 跑 Node.js。
风险：容器逃逸 → 攻击者可改写 .output 引入恶意代码，破坏供应链。
修复：运行阶段加 `USER node`（alpine 镜像内置 node 用户）。

P0-R2-9: 原 Dockerfile `rm -f package-lock.json` + `npm install`
→ 不可复现 + 供应链风险（每次 install 解析 latest）。

CL-06 Phase 0（2026-10-17）—— 本脚本自身修了两处**已实际发生**的失真：

  ① **假绿：断言命中注释**。旧断言 `"npm ci" in src` 之所以 exit 0，不是因为 Dockerfile 用了
     `npm ci`，而是因为第 13 行**注释**里写了 "npm ci"（`npm ci 保证构建可复现`）—— 而真实命令是
     `npm install`（第 20 行）。⇒ 现在先 `strip_comments()` 再断言（同理，负向自检会把"满足项写进注释"
     的变体断言为**必须 FAIL**，防复发）。

  ② **断言了错误的工具链**。仓库权威工具链是 **pnpm**：`package.json` 声明 `packageManager: pnpm@9`、
     `pnpm-lock.yaml` 才是实时锁（`package-lock.json` 停在 2026-09-24）、CI（`.github/workflows/web-test.yml`）
     用 `pnpm install --frozen-lockfile`。而 Dockerfile 走 npm 且 `npm install` 不做 frozen 解析
     ⇒ `npm error ERESOLVE`（vue-router@^5.4.0 vs nuxt@^4.2.1 的 peer 冲突）**谁都构建不出 web 镜像**
     （账本 `CL-F-28`）。⇒ 现在断言 Dockerfile 必须用 pnpm + `--frozen-lockfile`，并禁止 npm 安装路径。

字面量断言（**全部在去注释后的源码上进行**）：
  P0-R2-8 : prod 阶段含 `USER node`
  P0-R2-9 : 不再 `rm -f` 任何 lockfile
  P2-R2-16: 含 `HEALTHCHECK`
  CL06-1  : 安装用 `pnpm install --frozen-lockfile`；**禁用**用 npm 装**项目依赖**
            （允许 `npm i -g <tool>` 引导包管理器本身 —— 两件事，见下）
  CL06-2  : 复制 `pnpm-lock.yaml`
  CL06-3  : 构建用 pnpm（`pnpm run build` 或 `pnpm build`）
  CL06-4  : 基础镜像 Node 主版本 ≥ 22（nuxt@4.5 构建期需 `Set.prototype.difference`）

运行：
  python scripts/test_web_dockerfile_p0r28_r29.py            # 契约断言
  python scripts/test_web_dockerfile_p0r28_r29.py --selftest # 负向自检（防"命中注释"类假绿复发）
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


def strip_comments(src: str) -> str:
    """去掉整行注释（Dockerfile 里 `^\\s*#` 即注释行）。

    保留行号（用空行占位）以便报错定位；行尾注释不是 Dockerfile 语法，无需处理。
    """
    out = []
    for line in src.splitlines():
        out.append("" if re.match(r"^\s*#", line) else line)
    return "\n".join(out)


# 去注释后的源码上必须成立的断言（供契约与负向自检共用）
def assert_contract(code: str) -> list[str]:
    """返回违规项列表（空 = 通过）。入参必须是**已去注释**的源码。"""
    bad: list[str] = []

    from_matches = list(re.finditer(r"^FROM\s+\S+", code, re.MULTILINE))
    if len(from_matches) < 2:
        return [f"必须 ≥2 个 FROM（builder + runner），找到 {len(from_matches)} 个"]
    prod_section = code[from_matches[1].start():]

    if "USER node" not in prod_section:
        bad.append("P0-R2-8: prod 阶段缺 USER node（以 root 跑 Node.js）")
    if re.search(r"rm\s+-f\s+\S*lock", code):
        bad.append("P0-R2-9: 仍在 rm -f lockfile（不可复现）")
    if "HEALTHCHECK" not in code:
        bad.append("P2-R2-16: 缺 HEALTHCHECK")

    # CL06-1：必须用 pnpm 且 frozen lockfile；**项目依赖**的 npm 安装路径一律禁止。
    #   边界：允许 `npm i -g <tool>`（引导包管理器本身，如 `npm i -g pnpm@9.15.1`）——
    #   这与"用 npm 装**项目依赖**"是两件事，后者才是 CL-F-28 的根因（npm 侧解析与
    #   权威锁 pnpm-lock.yaml 不一致 ⇒ ERESOLVE）。判定用负向先行断言排除 -g/--global。
    if not re.search(r"pnpm\s+(install|i)\b[^\n]*--frozen-lockfile", code):
        bad.append("CL06-1: 未见 `pnpm install --frozen-lockfile`（构建不可复现）")
    if re.search(r"\bnpm\s+(install|i|ci)\b(?!\s+(?:-g|--global)\b)", code):
        bad.append("CL06-1: 仍存在用 npm 安装**项目依赖**的路径（与权威锁 pnpm-lock.yaml 不一致 ⇒ ERESOLVE）")

    # CL06-2：必须复制权威锁
    if "pnpm-lock.yaml" not in code:
        bad.append("CL06-2: 未复制 pnpm-lock.yaml")

    # CL06-3：构建必须走 pnpm
    if not re.search(r"pnpm\s+(run\s+)?build", code):
        bad.append("CL06-3: 未见 pnpm 构建命令（`pnpm run build` / `pnpm build`）")

    # CL06-4：基础镜像 Node 主版本必须 ≥ 22
    #   依据（实测 2026-10-17，CL-F-28 的第二段真因）：权威锁 pnpm-lock.yaml 解析出
    #   nuxt@4.5.2 / nitropack@2.13.4 / vite@8.3.0，构建期调用 `Set.prototype.difference`
    #   （Node 22+ 才有）⇒ 降回 node:20 会 `TypeError: trustedFunctions.difference is not a function`。
    majors = [int(m) for m in re.findall(r"node:(\d+)-alpine", code)]
    if not majors:
        bad.append("CL06-4: 未识别基础镜像的 node 主版本（期望形如 node:<N>-alpine）")
    elif min(majors) < 22:
        bad.append(f"CL06-4: 基础镜像 node 主版本 < 22（实为 {sorted(set(majors))}）⇒ nuxt 4.5 构建必失败")

    return bad


def main() -> int:
    if not WEB_DOCKERFILE.exists():
        print(f"FAIL  Dockerfile not found: {WEB_DOCKERFILE}")
        return 1
    src = WEB_DOCKERFILE.read_text(encoding="utf-8")
    code = strip_comments(src)

    print(f"=== {WEB_DOCKERFILE.name} 字面量契约（Stage 97 PR-4 + CL-06 Phase 0） ===")
    for item in assert_contract(code):
        check(item, False)
    if not failures:
        # 逐条打印 PASS（保持原有可读输出）
        print("  PASS  P0-R2-8: prod 阶段含 USER node")
        print("  PASS  P0-R2-9: 不再删除 lockfile")
        print("  PASS  P2-R2-16: 含 HEALTHCHECK")
        print("  PASS  CL06-1: 安装用 pnpm --frozen-lockfile（无 npm 安装路径）")
        print("  PASS  CL06-2: 复制 pnpm-lock.yaml")
        print("  PASS  CL06-3: 构建走 pnpm")
        print("  PASS  CL06-4: 基础镜像 node 主版本 ≥ 22")

    print()
    if failures:
        print(f"FAILED: {len(failures)} 项契约违规: {failures}")
        return 1
    print("OK: 全部字面量契约通过")
    return 0


def selftest() -> int:
    """负向自检：把"满足项"写进**注释**时，契约必须仍判 FAIL（防"命中注释"假绿复发）。

    同时校验去注释函数本身：满足项只在注释里出现 ⇒ strip 后不得残留。
    """
    print("=== 负向自检（注释不得充当证据） ===")
    cases = {
        "满足项全写在注释里（旧假绿形态）": (
            "# pnpm install --frozen-lockfile\n"
            "# pnpm-lock.yaml\n"
            "# pnpm run build\n"
            "# USER node\n"
            "# HEALTHCHECK\n"
            "FROM node:20-alpine AS builder\n"
            "RUN npm install\n"
            "FROM node:20-alpine\n"
        ),
        "去注释后仍缺 frozen-lockfile": (
            "FROM node:20-alpine AS builder\n"
            "COPY package.json pnpm-lock.yaml ./\n"
            "RUN pnpm install\n"  # 缺 --frozen-lockfile
            "RUN pnpm run build\n"
            "FROM node:20-alpine\n"
            "USER node\n"
            "HEALTHCHECK CMD true\n"
        ),
        "去注释后仍走 npm install（CL-F-28 形态）": (
            "FROM node:20-alpine AS builder\n"
            "COPY package.json pnpm-lock.yaml ./\n"
            "RUN pnpm install --frozen-lockfile\n"
            "RUN npm install --no-save foo\n"
            "RUN pnpm run build\n"
            "FROM node:20-alpine\n"
            "USER node\n"
            "HEALTHCHECK CMD true\n"
        ),
        "npm ci 装项目依赖（旧 P0-R2-9 想当然形态）": (
            "FROM node:20-alpine AS builder\n"
            "COPY package.json pnpm-lock.yaml ./\n"
            "RUN npm ci --omit=optional\n"
            "RUN pnpm run build\n"
            "FROM node:20-alpine\n"
            "USER node\n"
            "HEALTHCHECK CMD true\n"
        ),
        "基础镜像降回 node:20（CL-F-28 第二段真因）": (
            "FROM node:20-alpine AS builder\n"
            "COPY package.json pnpm-lock.yaml .npmrc ./\n"
            "RUN pnpm install --frozen-lockfile --ignore-scripts\n"
            "RUN pnpm run build\n"
            "FROM node:20-alpine\n"
            "USER node\n"
            "HEALTHCHECK CMD true\n"
        ),
    }
    bad_cases = 0
    for name, src in cases.items():
        violations = assert_contract(strip_comments(src))
        if violations:
            print(f"  PASS  自检[{name}] 被判违规（正确）: {violations[0]}")
        else:
            print(f"  FAIL  自检[{name}] **未被判违规** —— 契约存在假绿通道")
            bad_cases += 1

    # 正向：一个真合规的最小样例必须通过（证明契约不是"永远 FAIL"）；
    # 该样例用 `npm i -g pnpm` 引导工具（允许）+ pnpm 装项目依赖（要求）
    good = (
        "FROM node:22-alpine AS builder\n"
        "COPY package.json pnpm-lock.yaml .npmrc ./\n"
        "RUN npm i -g pnpm@9.15.1 --registry=https://registry.npmjs.org/ \\\n"
        "    && pnpm install --frozen-lockfile --ignore-scripts\n"
        "RUN pnpm run build\n"
        "FROM node:22-alpine\n"
        "USER node\n"
        "HEALTHCHECK CMD true\n"
    )
    v = assert_contract(strip_comments(good))
    if v:
        print(f"  FAIL  自检[合规样例应通过] 却报违规: {v}")
        bad_cases += 1
    else:
        print("  PASS  自检[合规样例] 通过（契约非恒 FAIL）")

    print()
    if bad_cases:
        print(f"FAILED: 负向自检 {bad_cases} 项不成立")
        return 1
    print("OK: 负向自检全部成立")
    return 0


if __name__ == "__main__":
    if "--selftest" in sys.argv:
        sys.exit(selftest())
    sys.exit(main())
