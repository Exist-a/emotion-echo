#!/usr/bin/env bash
# =====================================================
#  APISIX 初始路由推送脚本（traditional 模式）
#  用途：在 docker-compose up 之后执行，把基础路由推入 etcd
#  Windows 用法：将本文件内容复制到 PowerShell 终端逐条执行
# =====================================================

set -e

ADMIN_URL="http://localhost:9180/apisix/admin"
API_KEY="edd1c9f034335f136f87ad84b625c8f1"   # APISIX 3.x 默认 Admin API Key

echo "[1/4] 创建 upstream: emotion-echo-gin（指向宿主机 Gin :8080）"
curl -s -X PUT "$ADMIN_URL/upstreams/1" \
  -H "X-API-KEY: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "emotion-echo-gin",
    "type": "roundrobin",
    "nodes": [
      {"host": "host.docker.internal", "port": 8080, "weight": 1}
    ]
  }'
echo ""

echo "[2/4] 创建路由: /api/v1/* → upstream 1"
curl -s -X PUT "$ADMIN_URL/routes/1" \
  -H "X-API-KEY: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gin-default",
    "uri": "/api/v1/*",
    "methods": ["GET","POST","PUT","DELETE","PATCH"],
    "upstream_id": 1
  }'
echo ""

echo "[3/4] 创建 upstream: mock-server（APISIX 自带 :1980 的 mock 节点）"
curl -s -X PUT "$ADMIN_URL/upstreams/2" \
  -H "X-API-KEY: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "mock-server",
    "type": "roundrobin",
    "nodes": [
      {"host": "127.0.0.1", "port": 1980, "weight": 1}
    ]
  }'
echo ""

echo "[4/4] 创建路由: /ping → upstream 2（健康检查端点）"
curl -s -X PUT "$ADMIN_URL/routes/2" \
  -H "X-API-KEY: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping",
    "uri": "/ping",
    "upstream_id": 2
  }'
echo ""

echo "=== 推送完成 ==="
echo "测试："
echo "  curl http://localhost:9080/ping          # APISIX 内部 mock 服务（应有 hello 字符串）"
echo "  curl http://localhost:9080/api/v1/...    # 经网关到宿主机 Gin（需要 Gin 跑在 8080）"
