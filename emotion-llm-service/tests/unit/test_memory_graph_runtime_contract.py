"""CL-08c Phase 4 运行时修复的三点回归钉（源码字面量契约，先例：i011/i012 迁移测试）。

真栈调试发现的三个问题一旦回归即断链路，且都无法在无 graphiti 依赖的
单元环境里直接复现，故用字面量断言锁住修复点：
  1. json_object：deepseek 不支持 json_schema response_format（400）；
  2. 事件循环：初始化不得用 asyncio.run（会关闭循环、绑死 graphiti 原语）；
  3. 卷属主：Dockerfile 必须预建 /data/graph 并 chown（非 root 容器写命名卷）。
"""

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_llm_client_uses_json_object_mode():
    src = (ROOT / "memory_graph.py").read_text(encoding="utf-8")
    assert 'structured_output_mode="json_object"' in src, (
        "deepseek 不支持 json_schema response_format（实测 400）——"
        "去掉 json_object 模式会让全部图摄取失败"
    )


def test_init_does_not_use_asyncio_run_in_build():
    src = (ROOT / "memory_graph.py").read_text(encoding="utf-8")
    # _build 内不允许 asyncio.run（只有 bridge is None 的单测分支才允许）
    assert "asyncio.run(graphiti.build_indices" not in src.replace(
        "if bridge is not None:", "", 1
    ) or "bridge.call(graphiti.build_indices_and_constraints()" in src
    assert "bridge.call(graphiti.build_indices_and_constraints()" in src, (
        "初始化必须与后续操作同循环（AsyncBridge）；asyncio.run 会创建并关闭"
        "临时循环，graphiti 原语绑死死循环 ⇒ Event loop is closed"
    )


def test_grpc_server_starts_bridge_before_build():
    src = (ROOT / "grpc_server.py").read_text(encoding="utf-8")
    i_bridge = src.find("bridge = AsyncBridge()")
    i_start = src.find("bridge.start()")
    i_build = src.find("build_memory_graph_from_env(bridge)")
    assert 0 < i_bridge < i_start < i_build, (
        "装配顺序必须：先起事件循环线程，再在其上装配图（grpc_server serve()）"
    )


def test_dockerfile_precreates_graph_dir_with_owner():
    src = (ROOT / "Dockerfile").read_text(encoding="utf-8")
    assert "mkdir -p /data/graph" in src and "chown app:app /data/graph" in src, (
        "非 root 容器写命名卷：必须预建目录并 chown（卷首挂继承属主），"
        "否则 Permission denied ⇒ MemoryGraph 永久降级"
    )
