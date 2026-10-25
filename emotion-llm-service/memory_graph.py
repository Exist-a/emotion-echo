"""memory_graph.py — CL-08c Phase 4：图记忆（Graphiti + 嵌入式 FalkorDB）。

进程内拓扑（ADR 决策 45 · 技术细节 7）：
    ai-svc 抽取 worker --gRPC(UpsertEpisode)--> 本模块 --> Graphiti --> falkordblite

关键事实（Phase 0 spike 实测，evidence/phase0-t-e2*）：
    - falkordblite 的 Python 模块名是 **redislite**；`redislite.FalkorDB(dbfilename,
      serverconfig={'port': '8002'})` 会拉起自带 FalkorDB 模块的 redis-server；
    - graphiti 的 FalkorDriver **不能**直接吃同步实例，必须走 host/port TCP；
    - 多进程可共享同一 server（无 Kuzu 的单写者锁）；RDB 在 server 退出时落盘
      （SIGTERM 可靠，kill -9 丢最后窗口 ⇒ 图以 PG 为准、可重建）；
    - falkordblite 0.11.0 是 Beta；客户端无 shutdown()，生命周期由本模块管理。

配置（env，全部可缺省；缺关键项 ⇒ 模块不启用，gRPC 返回 ok=false 而非崩）：
    MEMORY_GRAPH_ENABLED      默认 true（依赖装不上时自动降级为禁用）
    MEMORY_GRAPH_DATA_DIR     默认 /data/graph（docker 挂命名卷）
    MEMORY_GRAPH_PORT         默认 8002（容器内固定，见 ADR 已知边界）
    MEMORY_GRAPH_DATABASE     默认 memory
    MEMORY_EMBED_BASE_URL     默认回落 TTS_API_BASE_URL（硅基流动，CL-D-36：不新增 key）
    MEMORY_EMBED_API_KEY      默认回落 TTS_API_KEY
    MEMORY_EMBED_MODEL        默认 BAAI/bge-m3（Phase 0 T-a 实测 1024 维）
    MEMORY_EMBED_DIM          默认 1024
    LLM_BASE_URL/LLM_API_KEY/LLM_MODEL    图摄取的 LLM（与 chat 主链路同一供给）
"""

from __future__ import annotations

import asyncio
import logging
import os
import threading
from datetime import datetime, timezone
from typing import Any, Protocol

logger = logging.getLogger("memory-graph")

DEFAULT_EMBED_MODEL = "BAAI/bge-m3"
DEFAULT_EMBED_DIM = 1024


class GraphitiLike(Protocol):
    """graphiti_core.Graphiti 的最小面（测试替身按此实现）。"""

    async def add_episode(
        self,
        name: str,
        episode_body: str,
        source_description: str,
        reference_time: datetime,
        source: Any = None,
        group_id: str | None = None,
        **kwargs: Any,
    ) -> Any: ...

    async def search(self, query: str, group_ids: list[str] | None = None, **kwargs: Any) -> list[Any]: ...

    async def remove_episode(self, episode_uuid: str) -> None: ...


def _env_bool(name: str, default: bool) -> bool:
    v = os.environ.get(name)
    if v is None:
        return default
    return v.strip().lower() not in ("0", "false", "no", "off")


class MemoryGraph:
    """图记忆的进程内门面：LLM/embedder 均为可注入接口（T10 决议：单测不打真 LLM）。"""

    def __init__(self, graphiti: GraphitiLike, episode_source: str = "message",
                 dropper=None, base_driver=None):
        self.g = graphiti
        self._episode_source = episode_source
        # dropper(group_id)：物理删掉一个 group 图（CL-08d 删全部快路径）；
        # graphiti 不暴露该能力 ⇒ 由装配层注入（redislite 客户端 GRAPH.DELETE）。
        self._dropper = dropper
        # base_driver：FalkorDriver 原始实例（CL-08d 删除路由用）。
        # ⚠️ 分图路由：graphiti 按 group_id 把每个用户的图存成**独立 graph key**
        #（Phase 4 T-17 实测），而 remove_episode 等节点级操作走 driver 的
        # 默认 database（'memory'，恒为空）⇒ 必须先 with_database(str(user_id))
        # 再执行，否则报 "node not found"（真栈实测踩过）。
        self._base_driver = base_driver

    async def upsert_episode(
        self,
        user_id: int,
        name: str,
        content: str,
        valid_at_unix_ms: int,
    ) -> tuple[bool, str]:
        if user_id <= 0:
            return False, "user_id must be positive"
        if not content or not content.strip():
            return False, "content must be non-empty"
        ref_time = (
            datetime.fromtimestamp(valid_at_unix_ms / 1000.0, tz=timezone.utc)
            if valid_at_unix_ms > 0
            else datetime.now(tz=timezone.utc)
        )
        try:
            result = await self.g.add_episode(
                name=name or content[:24],
                episode_body=content,
                source_description=self._episode_source,
                reference_time=ref_time,
                group_id=str(user_id),  # 用户隔离边界（T-17）
            )
            # CL-08d：回传 episode uuid（删除闭环 remove_episode 的定位键）。
            uuid = getattr(result, "episode", None)
            uuid = getattr(uuid, "uuid", "") if uuid is not None else ""
            return True, str(uuid or "")
        except Exception as exc:  # noqa: BLE001 —— 图失败必须被服务端折算成 ok=false
            logger.warning("memory-graph upsert failed: %s", exc)
            return False, f"graphiti add_episode: {exc}"

    async def remove_episode(self, user_id: int, episode_uuid: str) -> tuple[bool, str]:
        """按 uuid 摘掉一条情节（CL-08d 删除闭环；ADR 决策 3：图随删）。

        分图路由：remove 只做图查询/删除（无 LLM），用一个绑到该用户 group 图的
        临时 Graphiti 执行（复用上游 remove_episode 的边/孤儿节点清理逻辑）。
        base_driver 未注入（单测假实现）⇒ 直接走 self.g（假实现自行处理路由）。
        """
        if user_id <= 0 or not episode_uuid:
            return False, "user_id and episode_uuid are required"
        try:
            target = self.g
            if self._base_driver is not None:
                from graphiti_core.graphiti import Graphiti

                target = Graphiti(
                    graph_driver=self._base_driver.with_database(str(user_id)),
                    llm_client=getattr(self.g, "llm_client", None),
                    embedder=getattr(self.g, "embedder", None),
                    cross_encoder=getattr(self.g, "cross_encoder", None),
                )
            await target.remove_episode(episode_uuid)
            if self._base_driver is not None:
                # 上游 remove_episode 会留下孤儿实体（边清了、节点残留 —— falkordblite
                # 0.11.0 实测）：0 条 MENTIONS 入边的实体 = 无任何情节支持 ⇒ 清除，
                # 保证"图上不可再检索"的闭环（T-16）。仅真实装配路径执行（base_driver
                # 注入）；单测假实现走 fallback，无图可扫。
                await self._base_driver.with_database(str(user_id)).execute_query(
                    "MATCH (n:Entity) WHERE NOT ( ()-[:MENTIONS]->(n) ) DELETE n"
                )
            return True, ""
        except Exception as exc:  # noqa: BLE001
            logger.warning("memory-graph remove failed: %s", exc)
            return False, f"graphiti remove_episode: {exc}"

    async def delete_user_data(self, user_id: int) -> tuple[bool, str]:
        """删除用户全部图数据（快路径：graphiti 按 group_id 物理分图 ⇒ drop 该图）。

        图不存在 = 本来就没有 ⇒ 视为成功（幂等闭环）。
        """
        if user_id <= 0:
            return False, "user_id must be positive"
        if self._dropper is None:
            return False, "dropper not configured"
        try:
            await asyncio.to_thread(self._dropper, str(user_id))
            return True, ""
        except Exception as exc:  # noqa: BLE001
            msg = str(exc)
            if "not exist" in msg.lower() or "no such" in msg.lower():
                return True, ""
            logger.warning("memory-graph delete_group failed: %s", exc)
            return False, f"drop graph: {exc}"

    async def search(self, query: str, user_id: int, limit: int = 8) -> list[Any]:
        """三层召回（向量 + 全文 + 图遍历，Graphiti 的 combined hybrid）。

        group_ids 过滤是硬边界：检索结果绝不跨用户（T-17）。
        """
        if user_id <= 0 or not query.strip():
            return []
        try:
            return await self.g.search(query=query, group_ids=[str(user_id)], num_results=limit)
        except Exception as exc:  # noqa: BLE001
            logger.warning("memory-graph search failed: %s", exc)
            return []


# ---------------------------------------------------------------------------
# 装配（from_env）：真实 Graphiti + falkordblite。任何一步失败 ⇒ 返回 None，
# 调用方（grpc_server）把服务面降级为「返回 ok=false」，进程不崩（记忆是增强不是依赖）。
# ---------------------------------------------------------------------------


def build_memory_graph_from_env(bridge: "AsyncBridge | None" = None) -> MemoryGraph | None:
    """装配真实图后端。bridge 必须已 start：初始化（建索引）与后续所有图操作
    必须跑在**同一个**事件循环上 —— 用 asyncio.run 做初始化会创建并关闭一个
    临时循环，graphiti/falkordb 的 asyncio 原语会绑死那个死循环，之后一切
    操作报 "Event loop is closed"（真栈实测）。
    """
    if not _env_bool("MEMORY_GRAPH_ENABLED", True):
        logger.info("memory-graph disabled by env")
        return None
    try:
        return _build(bridge)
    except Exception as exc:  # noqa: BLE001
        logger.warning("memory-graph unavailable (degraded): %s", exc)
        return None


def _build(bridge: "AsyncBridge | None") -> MemoryGraph:
    # 延迟 import：graphiti/falkordblite 未装（如单元环境）时本函数不可调用而非 import 失败
    from graphiti_core.driver.falkordb_driver import FalkorDriver
    from graphiti_core.embedder.openai import OpenAIEmbedder, OpenAIEmbedderConfig
    from graphiti_core.llm_client.config import LLMConfig
    from graphiti_core.llm_client.openai_generic_client import OpenAIGenericClient
    from graphiti_core.cross_encoder.openai_reranker_client import OpenAIRerankerClient
    from graphiti_core.graphiti import Graphiti
    from redislite import FalkorDB as LiteFalkorDB

    data_dir = os.environ.get("MEMORY_GRAPH_DATA_DIR", "/data/graph").strip()
    port = int(os.environ.get("MEMORY_GRAPH_PORT", "8002"))
    database = os.environ.get("MEMORY_GRAPH_DATABASE", "memory").strip() or "memory"

    os.makedirs(data_dir, exist_ok=True)
    db_file = os.path.join(data_dir, "graph.db")

    # 拉起嵌入式 FalkorDB（redis-server 子进程，宿主端口固定 —— spike 实测多进程共享同一 server）
    lite = LiteFalkorDB(dbfilename=db_file, serverconfig={"port": str(port)})

    def _drop_group(group_id: str) -> None:
        # CL-08d：GRAPH.DELETE 物理删一个 group 图（不存在时由上层幂等化）
        lite.execute_command("GRAPH.DELETE", group_id)

    driver = FalkorDriver(host="127.0.0.1", port=port, database=database)

    llm_key = os.environ.get("LLM_API_KEY", "").strip()
    llm_cfg = LLMConfig(
        api_key=llm_key or None,
        model=os.environ.get("LLM_MODEL", "").strip() or None,
        base_url=os.environ.get("LLM_BASE_URL", "").strip() or None,
    )
    embed_base = os.environ.get("MEMORY_EMBED_BASE_URL", "").strip() or os.environ.get("TTS_API_BASE_URL", "").strip()
    embed_key = os.environ.get("MEMORY_EMBED_API_KEY", "").strip() or os.environ.get("TTS_API_KEY", "").strip()

    embedder = OpenAIEmbedder(
        config=OpenAIEmbedderConfig(
            embedding_model=os.environ.get("MEMORY_EMBED_MODEL", DEFAULT_EMBED_MODEL),
            embedding_dim=int(os.environ.get("MEMORY_EMBED_DIM", str(DEFAULT_EMBED_DIM))),
            api_key=embed_key or None,
            base_url=embed_base or None,
        )
    )

    # structured_output_mode：deepseek 不支持 json_schema response_format
    #（实测 400 "This response_format type is unavailable now"，直测 json_object 可用）
    # ⇒ 用 json_object + prompt 注入 schema（generic client 内建行为）。
    graphiti = Graphiti(
        graph_driver=driver,
        llm_client=OpenAIGenericClient(config=llm_cfg, structured_output_mode="json_object"),
        embedder=embedder,
        cross_encoder=OpenAIRerankerClient(config=llm_cfg),
    )
    graph = MemoryGraph(graphiti, dropper=_drop_group, base_driver=driver)

    # 建索引/约束（幂等；spike 实测 falkordb 驱动支持）。
    # 必须与后续操作同循环：bridge 已启动 ⇒ 在它的循环上跑；未提供 bridge
    # （单测/探针）⇒ 退回 asyncio.run（此时进程内不会有后续图操作）。
    if bridge is not None:
        bridge.call(graphiti.build_indices_and_constraints(), timeout=30.0)
    else:
        asyncio.run(graphiti.build_indices_and_constraints())
    logger.info(
        "memory-graph ready: db=%s port=%d embed_model=%s", db_file, port,
        os.environ.get("MEMORY_EMBED_MODEL", DEFAULT_EMBED_MODEL),
    )
    return graph


# ---------------------------------------------------------------------------
# sync→async 桥：grpc_server 是同步 grpc.server；graphiti 是 asyncio。
# 专用事件循环线程承接所有图操作（同线程串行，天然避免图侧并发写竞争）。
# ---------------------------------------------------------------------------


class AsyncBridge:
    def __init__(self) -> None:
        self._loop: asyncio.AbstractEventLoop | None = None  # type: ignore[valid-type]
        self._thread: threading.Thread | None = None
        self._started = threading.Event()

    def start(self) -> None:
        if self._thread is not None:
            return
        self._thread = threading.Thread(target=self._run, name="memory-graph-loop", daemon=True)
        self._thread.start()
        if not self._started.wait(timeout=10):
            raise RuntimeError("memory-graph loop failed to start")

    def _run(self) -> None:
        loop = asyncio.new_event_loop()
        self._loop = loop
        asyncio.set_event_loop(loop)
        self._started.set()
        loop.run_forever()

    def call(self, coro_factory, timeout: float = 30.0):
        """在图事件循环上执行协程并同步等待结果。"""
        import concurrent.futures

        if self._loop is None or not self._loop.is_running():
            raise RuntimeError("memory-graph loop not running")
        fut = asyncio.run_coroutine_threadsafe(coro_factory, self._loop)
        try:
            return fut.result(timeout=timeout)
        except concurrent.futures.TimeoutError as exc:
            fut.cancel()
            raise TimeoutError(f"memory-graph op timed out after {timeout}s") from exc
