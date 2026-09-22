#!/usr/bin/env bash
# verify.sh — SenseVoice 镜像修复端到端验证（不依赖 docker daemon 抖）
#
# 背景：E2E-F-106 修复后需要端到端确认
#   ① /analyze 真实推理返回 transcript
#   ② 容器 RestartCount 不增长（不重启）
# Docker Desktop 在嵌套 shell 间歇 500，但 buildx / docker run / docker exec 仍可走 —
# 我们使用与"嵌套 shell 调用"不同的路径：
#   - 不用 MSYS_NO_PATHCONV 之外的转义
#   - 用 docker compose 调用（不走 docker daemon HTTP API 的命名 pipe）
#   - 不并发（避免多个后台 docker CLI 互相干扰）
#
# 用法（Docker 恢复后）：
#   cd deploy && bash ../emotion-echo-models/SV-fastbuild/verify.sh

set -uo pipefail

CONV=tmp/e2e16/clip.wav

echo "=== 1. 当前 SenseVoice 容器状态 ==="
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev \
  ps emotion-echo-sensevoice 2>&1 | tail -5

echo
echo "=== 2. 把 clip.wav 拷进容器 ==="
# 使用 compose exec（与 docker exec 不同代码路径，不走命名 pipe）
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev \
  cp emotion-echo-sensevoice:../tmp/clip.wav /tmp/clip.wav 2>&1 | tail -3

echo
echo "=== 3. 在容器内 /analyze（30~60s 应回） ==="
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev \
  exec -T emotion-echo-sensevoice bash -c \
    'curl -s -m 120 -X POST http://localhost:8002/analyze \
     -F "file=@/tmp/clip.wav;type=audio/wav" -w "\nhttp=%{http_code} time=%{time_total}s\n"' 2>&1 | tail -10

echo
echo "=== 4. RestartCount（修复前会累加；修复后应=0） ==="
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev \
  ps emotion-echo-sensevoice --format json 2>&1 | grep -oE '"RestartCount":[0-9]+' | head -1
