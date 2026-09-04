"""emotion-llm-service · Dockerfile 打包完整性契约

背景：Dockerfile 两个阶段都是逐个文件 COPY（没有 `COPY . .`），漏掉任何一个
被 import 的本地模块，容器都会在启动瞬间 ModuleNotFoundError 然后进入无限
重启。Stage 39 就是这样漏了 nacos_client.py —— requirements.txt 里的
nacos-sdk-python 装上了，但项目自己的 nacos_client.py 从没进镜像。

本契约不针对单个文件名，而是覆盖整类遗漏：
main.py / grpc_server.py import 的每个**本地**模块（即服务目录下同名 .py），
必须同时出现在 builder 阶段和 runtime 阶段的 COPY 里。
"""
from __future__ import annotations

import ast
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
DOCKERFILE = SERVICE_DIR / "Dockerfile"

# 容器 ENTRYPOINT 实际拉起的两个入口（entrypoint.sh 分别起 HTTP 和 gRPC 进程）
ENTRYPOINTS = ("main.py", "grpc_server.py")


def _local_imports(py_path: Path) -> set[str]:
    """返回 py_path 依赖的本地模块名（能在服务目录下找到同名 .py 的那些）。"""
    tree = ast.parse(py_path.read_text(encoding="utf-8"))
    names: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                names.add(alias.name.split(".")[0])
        elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module:
            names.add(node.module.split(".")[0])
    return {n for n in names if (SERVICE_DIR / f"{n}.py").exists()}


def _stage_sections() -> tuple[str, str]:
    """把 Dockerfile 切成 builder / runtime 两段文本。"""
    text = DOCKERFILE.read_text(encoding="utf-8")
    lines = text.splitlines()
    runtime_start = next(
        i for i, line in enumerate(lines)
        if line.startswith("FROM") and "AS runtime" in line
    )
    builder_start = next(
        i for i, line in enumerate(lines)
        if line.startswith("FROM") and "AS builder" in line
    )
    return (
        "\n".join(lines[builder_start:runtime_start]),
        "\n".join(lines[runtime_start:]),
    )


def _required_modules() -> set[str]:
    required: set[str] = set()
    for entry in ENTRYPOINTS:
        required.add(entry[: -len(".py")])
        required |= _local_imports(SERVICE_DIR / entry)
    return required


@pytest.mark.parametrize("stage", ["builder", "runtime"])
def test_dockerfile_copies_every_local_module(stage: str) -> None:
    builder_section, runtime_section = _stage_sections()
    section = builder_section if stage == "builder" else runtime_section

    missing = sorted(
        f"{mod}.py" for mod in _required_modules() if f"{mod}.py" not in section
    )

    assert not missing, (
        f"{stage} 阶段漏 COPY 这些被 import 的本地模块：{missing}。"
        f"漏任何一个都会让容器启动即 ModuleNotFoundError 并无限重启。"
    )


def test_local_imports_detects_nacos_client() -> None:
    """守住检测逻辑本身：main.py 确实 import 了 nacos_client（否则上面的契约会空跑）。"""
    assert "nacos_client" in _local_imports(SERVICE_DIR / "main.py")
