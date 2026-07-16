#!/bin/sh
# emotion-llm-service entrypoint (POSIX /bin/sh 兼容)
# 后台启动 uvicorn (HTTP :8000) + grpc_server (gRPC :50051)
# 任一进程退出，整个容器退出（tini/PID 1 接管）
#
# 注意：python:3.12-slim 用 dash，没有 `wait -n`，改用 trap 监控

set -e

HTTP_PORT="${HTTP_PORT:-8000}"
GRPC_PORT="${GRPC_PORT:-50051}"

echo "[entrypoint] starting emotion-llm-service: HTTP=${HTTP_PORT} gRPC=${GRPC_PORT}"

# 后台启动 HTTP (FastAPI)
python main.py &
HTTP_PID=$!
echo "[entrypoint] HTTP pid=${HTTP_PID}"

# 后台启动 gRPC
python grpc_server.py &
GRPC_PID=$!
echo "[entrypoint] gRPC pid=${GRPC_PID}"

# 任意子进程退出 → 整体退出（容器会被 orchestrator 重启）
cleanup() {
    echo "[entrypoint] shutting down..."
    kill -TERM ${HTTP_PID} ${GRPC_PID} 2>/dev/null || true
    wait ${HTTP_PID} ${GRPC_PID} 2>/dev/null || true
}
trap cleanup TERM INT

# 监控：用 wait 等待任一子进程退出（dash sh 没有 wait -n 但 wait 会阻塞直到所有子进程退出）
# 退而求其次：循环轮询，每 1 秒检查子进程状态
while true; do
    # 检查 HTTP_PID 是否还在
    if ! kill -0 ${HTTP_PID} 2>/dev/null; then
        echo "[entrypoint] HTTP process (pid=${HTTP_PID}) exited"
        cleanup
        exit 1
    fi
    # 检查 GRPC_PID 是否还在
    if ! kill -0 ${GRPC_PID} 2>/dev/null; then
        echo "[entrypoint] gRPC process (pid=${GRPC_PID}) exited"
        cleanup
        exit 1
    fi
    sleep 1
done
