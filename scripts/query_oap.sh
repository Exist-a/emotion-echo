#!/usr/bin/env bash
# scripts/query_oap.sh — E2E-26 #8：SkyWalking OAP GraphQL 查询工具
#
# 三条硬契约（scripts/test_query_oap.sh 守卫）：
#   C1 Duration SECOND 步长格式 = yyyy-MM-dd HHmmss（**无冒号**，官方
#      apache/skywalking-query-protocol common.graphqls；带冒号即 OAP 校验错误——
#      历史「OAP 9.x queryDuration bug」的真因候选，M1 定性用）
#   C2 访问走 docker network 直达 skywalking-oap:12800 —— 宿主 12800 无端口映射
#      （Stage 33 端口收紧，F-c），宿主 curl 恒超时
#   C4 失败即非零退出：curl 失败 / 响应含 errors 都算失败（契约 9 式「探测失败
#      只 WARN 不计 FAIL」的吞错是反面教材）
#
# 用法：
#   bash scripts/query_oap.sh services                     # 服务列表（含 id）
#   bash scripts/query_oap.sh traces <服务名或id> [分钟]    # queryBasicTraces
#   bash scripts/query_oap.sh trace <traceId>              # queryTrace 全 span
#   bash scripts/query_oap.sh logs [分钟]                  # queryLogs（APISIX 上报）
#   bash scripts/query_oap.sh raw '<GraphQL query>'        # 原样查询
#   bash scripts/query_oap.sh dry-run traces <服务名> [分钟] # 只打印将发送的 JSON
#   bash scripts/query_oap.sh format-duration [epoch]      # 纯函数（测试用）
#
# 退出码：0 成功；1 查询失败/响应含 errors；2 用法错误。

set -uo pipefail

usage() {
  sed -n '3,20p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

# C1：官方 SECOND 步长格式（无冒号）
format_duration() {
  local epoch="${1:-$(date +%s)}"
  date -d "@${epoch}" '+%Y-%m-%d %H%M%S'
}

oap_net() {
  local net
  net=$(docker inspect emotion-echo-sw-oap --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}' 2>/dev/null | awk '{print $1}')
  if [ -z "$net" ]; then
    echo "query_oap: sw-oap 容器不可达（未启动？）" >&2
    exit 1
  fi
  printf '%s' "$net"
}

# 发送查询；响应含 "errors" → 打印到 stderr 并返回 1（C4）
oap_graphql() {
  local payload="$1"
  local net resp
  net=$(oap_net)
  if ! resp=$(docker run --rm --network "$net" curlimages/curl:latest \
        -sS -m 15 -X POST http://skywalking-oap:12800/graphql \
        -H 'Content-Type: application/json' \
        -d "$payload" 2>&1); then
    echo "query_oap: curl 请求失败: $resp" >&2
    return 1
  fi
  if printf '%s' "$resp" | grep -q '"errors"'; then
    echo "query_oap: GraphQL 错误响应: $resp" >&2
    return 1
  fi
  printf '%s\n' "$resp"
}

# 服务名 → serviceId（metadata-v2 规则：BASE64(name) + '.1'）；已是 id 则原样
service_id() {
  local s="$1"
  case "$s" in
    *.1) printf '%s' "$s" ;;
    *=.1) printf '%s' "$s" ;;
    *) printf '%s' "$(printf '%s' "$s" | base64 | tr -d '\n').1" ;;
  esac
}

window() {
  local minutes="${1:-10}"
  local now start
  now=$(date +%s)
  start=$(format_duration $((now - minutes * 60)))
  END_T=$(format_duration "$now")
  START_T="$start"
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    format-duration)
      format_duration "${2:-}"
      ;;
    services)
      oap_graphql '{"query":"{ listServices { id name normal } }"}'
      ;;
    traces)
      [ $# -ge 2 ] || usage
      window "${3:-10}"
      local sid
      sid=$(service_id "$2")
      oap_graphql "{\"query\":\"{ queryBasicTraces(condition: {serviceId: \\\"$sid\\\", queryDuration: {start: \\\"$START_T\\\", end: \\\"$END_T\\\", step: SECOND}, traceState: ALL, queryOrder: BY_START_TIME, paging: {pageNum: 1, pageSize: 50}}) { traces { traceIds endpointNames isError start } } }\"}"
      ;;
    trace)
      [ $# -ge 2 ] || usage
      oap_graphql "{\"query\":\"{ queryTrace(traceId: \\\"$2\\\") { spans { traceId segmentId spanId parentSpanId refs { traceId parentSegmentId parentSpanId type } serviceCode endpointName type peer isError tags { key value } } } }\"}"
      ;;
    logs)
      window "${2:-10}"
      oap_graphql "{\"query\":\"{ queryLogs(condition: {queryDuration: {start: \\\"$START_T\\\", end: \\\"$END_T\\\", step: SECOND}, paging: {pageNum: 1, pageSize: 30}}) { logs { timestamp traceId contentType content } } }\"}"
      ;;
    dry-run)
      # 只打印将发送的 payload（契约审计用，不发请求、不碰 docker）
      shift
      local sub="${1:-}"
      case "$sub" in
        traces)
          [ $# -ge 2 ] || usage
          window "${3:-10}"
          local dsid
          dsid=$(service_id "$2")
          printf '{"query":"{ queryBasicTraces(condition: {serviceId: \\"%s\\", queryDuration: {start: \\"%s\\", end: \\"%s\\", step: SECOND}, traceState: ALL, queryOrder: BY_START_TIME, paging: {pageNum: 1, pageSize: 50}}) { traces { traceIds endpointNames isError start } } }"}\n' \
            "$dsid" "$START_T" "$END_T"
          ;;
        *) usage ;;
      esac
      ;;
    raw)
      [ $# -ge 2 ] || usage
      oap_graphql "{\"query\":\"$2\"}"
      ;;
    *)
      usage
      ;;
  esac
}

main "$@"
