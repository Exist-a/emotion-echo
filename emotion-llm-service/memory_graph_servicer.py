"""memory_graph_servicer.py — CL-08c Phase 4：MemoryGraphService 的 gRPC 实现。

生命周期：由 grpc_server.serve() 装配（graph=None 时降级 —— 恒返回 ok=false，
不崩进程）；graphiti 是 asyncio，经 AsyncBridge 在专用事件循环线程上执行。
"""

from __future__ import annotations

import logging
import time

from memory_graph import AsyncBridge, MemoryGraph

import memory_graph_pb2
import memory_graph_pb2_grpc

logger = logging.getLogger("grpc-server")

# 单次图摄取的上限（LLM 摄取管线 + embedder 多轮调用；超时按失败处理，不挂线程池）
MEMORY_GRAPH_RPC_TIMEOUT_SEC = 60.0


class MemoryGraphServicer(memory_graph_pb2_grpc.MemoryGraphServiceServicer):
    def __init__(self, graph: MemoryGraph | None, bridge: AsyncBridge | None):
        self._graph = graph
        self._bridge = bridge

    def UpsertEpisode(self, request, context):
        started = time.time()
        if self._graph is None or self._bridge is None:
            logger.warning(
                "[grpc-server] UpsertEpisode in DEGRADED mode (graph unavailable): "
                "source=%s/%s user=%s",
                request.source_kind, request.source_id, request.user_id,
            )
            return memory_graph_pb2.UpsertEpisodeResponse(
                ok=False, message="memory graph unavailable (degraded)"
            )
        try:
            ok, detail = self._bridge.call(
                self._graph.upsert_episode(
                    user_id=request.user_id,
                    name=request.name,
                    content=request.content,
                    valid_at_unix_ms=request.valid_at_unix_ms,
                ),
                timeout=MEMORY_GRAPH_RPC_TIMEOUT_SEC,
            )
        except TimeoutError as exc:
            ok, detail = False, str(exc)
        except Exception as exc:  # noqa: BLE001 —— 图失败折算 ok=false，不崩服务
            ok, detail = False, f"memory graph rpc: {exc}"
        # detail 语义（memory_graph.upsert_episode）：成功 = episode uuid；失败 = 错误消息
        message = "" if ok else detail
        episode_uuid = detail if ok else ""
        if not ok:
            logger.warning(
                "[grpc-server] UpsertEpisode failed user=%s source=%s/%s: %s",
                request.user_id, request.source_kind, request.source_id, message,
            )
        logger.info(
            "[grpc-server] UpsertEpisode user=%s source=%s/%s ok=%s took=%.1fs",
            request.user_id, request.source_kind, request.source_id, ok,
            time.time() - started,
        )
        return memory_graph_pb2.UpsertEpisodeResponse(
            ok=ok, message=message or "", episode_uuid=episode_uuid,
        )

    def DeleteUserData(self, request, context):
        if self._graph is None or self._bridge is None:
            return memory_graph_pb2.DeleteUserDataResponse(
                ok=False, message="memory graph unavailable (degraded)"
            )
        try:
            ok, detail = self._bridge.call(
                self._graph.delete_user_data(user_id=request.user_id),
                timeout=MEMORY_GRAPH_RPC_TIMEOUT_SEC,
            )
        except TimeoutError as exc:
            ok, detail = False, str(exc)
        except Exception as exc:  # noqa: BLE001
            ok, detail = False, f"memory graph rpc: {exc}"
        logger.info(
            "[grpc-server] DeleteUserData user=%s ok=%s %s",
            request.user_id, ok, "" if ok else detail,
        )
        return memory_graph_pb2.DeleteUserDataResponse(ok=ok, message=detail if not ok else "")

    def RemoveEpisode(self, request, context):
        if self._graph is None or self._bridge is None:
            return memory_graph_pb2.RemoveEpisodeResponse(
                ok=False, message="memory graph unavailable (degraded)"
            )
        try:
            ok, detail = self._bridge.call(
                self._graph.remove_episode(
                    user_id=request.user_id,
                    episode_uuid=request.episode_uuid,
                ),
                timeout=MEMORY_GRAPH_RPC_TIMEOUT_SEC,
            )
        except TimeoutError as exc:
            ok, detail = False, str(exc)
        except Exception as exc:  # noqa: BLE001
            ok, detail = False, f"memory graph rpc: {exc}"
        logger.info(
            "[grpc-server] RemoveEpisode user=%s ok=%s %s",
            request.user_id, ok, "" if ok else detail,
        )
        return memory_graph_pb2.RemoveEpisodeResponse(ok=ok, message=detail if not ok else "")


def _make_memory_graph_servicer(graph, bridge):
    return MemoryGraphServicer(graph, bridge)
