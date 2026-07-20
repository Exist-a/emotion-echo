#!/usr/bin/env bash
# =====================================================
#  APISIX 初始路由推送脚本 (Stage 26-P)
#  适用模式: traditional + etcd (APISIX 3.9 实际可行)
#  跑法: docker compose -f deploy/docker-compose.infra.yml up -d 后,
#        bash deploy/apisix/seed.sh
# =====================================================

set -e

ADMIN_URL="http://localhost:9180/apisix/admin"
API_KEY="edd1c9f034335f136f87ad84b625c8f1"

post() {
  curl -sS -X PUT "$ADMIN_URL/$1/$2" \
    -H "X-API-KEY: $API_KEY" \
    -H "Content-Type: application/json" \
    -d "$3"
  echo ""
}

list() {
  curl -sS "$ADMIN_URL/$1" -H "X-API-KEY: $API_KEY"
}

del() {
  curl -sS -X DELETE "$ADMIN_URL/$1/$2" -H "X-API-KEY: $API_KEY"
  echo ""
}

echo "[*] 删除旧 upstream / route..."
for id in $(list upstreams | python -c 'import sys,json; print(" ".join(str(x["value"]["id"]) for x in json.load(sys.stdin).get("list",[]) or []))'); do
  del upstreams "$id"
done
for id in $(list routes | python -c 'import sys,json; print(" ".join(str(x["value"]["id"]) for x in json.load(sys.stdin).get("list",[]) or []))'); do
  del routes "$id"
done

echo "[*] Stage 26-P · 6 upstream + 16 route 推送..."

# ---------- 6 UPSTREAM ----------
post upstreams 1 '{
  "name": "user-svc",
  "type": "roundrobin",
  "nodes": [{"host": "emotion-echo-user-svc", "port": 8888, "weight": 1}]
}'

post upstreams 2 '{
  "name": "chat-svc",
  "type": "roundrobin",
  "nodes": [{"host": "emotion-echo-chat-svc", "port": 8890, "weight": 1}]
}'

post upstreams 3 '{
  "name": "analytics-svc",
  "type": "roundrobin",
  "nodes": [{"host": "emotion-echo-analytics-svc", "port": 8893, "weight": 1}]
}'

post upstreams 4 '{
  "name": "assessment-svc",
  "type": "roundrobin",
  "nodes": [{"host": "emotion-echo-assessment-svc", "port": 8889, "weight": 1}]
}'

post upstreams 5 '{
  "name": "ai-svc",
  "type": "roundrobin",
  "nodes": [{"host": "emotion-echo-ai-svc", "port": 8891, "weight": 1}]
}'

post upstreams 6 '{
  "name": "mock-ping",
  "type": "roundrobin",
  "nodes": [{"host": "127.0.0.1", "port": 1980, "weight": 1}]
}'

# ---------- 16 ROUTE ----------
post routes r-user-me '{
  "name": "r-user-me",
  "uri": "/api/v1/users/me",
  "methods": ["GET"],
  "upstream_id": 1
}'

post routes r-user-by-id '{
  "name": "r-user-by-id",
  "uri": "/api/v1/users/:id",
  "methods": ["GET"],
  "upstream_id": 1
}'

post routes r-user-update '{
  "name": "r-user-update",
  "uri": "/api/v1/users/me",
  "methods": ["PATCH"],
  "upstream_id": 1
}'

post routes r-conv-create '{
  "name": "r-conv-create",
  "uri": "/api/v1/conversations",
  "methods": ["POST"],
  "upstream_id": 2
}'

post routes r-msg-list '{
  "name": "r-msg-list",
  "uri": "/api/v1/conversations/*/messages",
  "methods": ["GET"],
  "upstream_id": 2
}'

post routes r-msg-send '{
  "name": "r-msg-send",
  "uri": "/api/v1/conversations/*/messages",
  "methods": ["POST"],
  "upstream_id": 2
}'

post routes r-analytics-health '{
  "name": "r-analytics-health",
  "uri": "/analytics-health",
  "upstream_id": 3
}'

post routes r-surveys '{
  "name": "r-surveys",
  "uri": "/api/v1/surveys",
  "methods": ["GET"],
  "upstream_id": 4
}'

post routes r-survey-get '{
  "name": "r-survey-get",
  "uri": "/api/v1/surveys/*",
  "methods": ["GET"],
  "upstream_id": 4
}'

post routes r-survey-submit '{
  "name": "r-survey-submit",
  "uri": "/api/v1/surveys/*/submit",
  "methods": ["POST"],
  "upstream_id": 4
}'

post routes r-survey-results-list '{
  "name": "r-survey-results-list",
  "uri": "/api/v1/surveys/results",
  "methods": ["GET"],
  "upstream_id": 4
}'

post routes r-survey-results-get '{
  "name": "r-survey-results-get",
  "uri": "/api/v1/surveys/results/*",
  "methods": ["GET"],
  "upstream_id": 4
}'

post routes r-emotion-by-msg '{
  "name": "r-emotion-by-msg",
  "uri": "/api/v1/emotion/message/*",
  "methods": ["GET"],
  "upstream_id": 5
}'

post routes r-emotion-by-conv '{
  "name": "r-emotion-by-conv",
  "uri": "/api/v1/emotion/conversation/*",
  "methods": ["GET"],
  "upstream_id": 5
}'

post routes r-ai-health '{
  "name": "r-ai-health",
  "uri": "/ai-health",
  "upstream_id": 5
}'

post routes r-ping '{
  "name": "r-ping",
  "uri": "/ping",
  "upstream_id": 6
}'

echo "=== 推送完成 ==="
echo "测试: curl http://localhost:9080/ping"
