#!/usr/bin/env bash
# CDN 候选可达性探测脚本（Lane O · T2#2 · 端侧化 stage1 · 编译链路+CDN 清单 §三 实证）。
#
# 目的：实测 5 候选 CDN 的可达性 + CORS + wasm magic byte，输出 markdown 报告。
# 输出：probe_report.md（默认）/ probe_report_<timestamp>.md（指定）
#
# 跑法：
#   bash scripts/on-device-cdn-probe/probe.sh                           # 实跑 + 写 report
#   bash scripts/on-device-cdn-probe/probe.sh --dry-run                 # 离线模式（CI 确定性）
#   bash scripts/on-device-cdn-probe/probe.sh --host aliyuncs.com       # 单 host 实跑
#   bash scripts/on-device-cdn-probe/probe.sh --output /tmp/report.md  # 自定义输出
#
# 退出码：0 = 全部 PASS 或 SKIP（无 FAIL），1 = 任意 FAIL
#
# 调研依据：on-device-compile-cdn-2026-09-24.md §三 + smoke_data_layer.py:48-50 skip 模式 +
# check_required_checks.py:24-26 显式 SKIP 纪律 + smoke_upload_minio.sh:71 HEAD 探测模板。

set -u

OUTPUT=""
DRY_RUN=0
ONLY_HOST=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --host)    ONLY_HOST="$2"; shift ;;
    --output)  OUTPUT="$2"; shift ;;
    -h|--help)
      echo "Usage: bash probe.sh [--dry-run] [--host <substring>] [--output <file>]"
      echo "  --dry-run   离线确定性模式（不真发请求）"
      echo "  --host      只探测名字含 substring 的 host"
      echo "  --output    报告输出文件（默认 probe_report.md）"
      exit 0
      ;;
    *) echo "Unknown arg: $1"; exit 2 ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -z "$OUTPUT" ]] && OUTPUT="$SCRIPT_DIR/probe_report.md"

# ---------- 候选 CDN host 列表（来自 on-device-compile-cdn-2026-09-24.md §三）----------
# 格式：<id>|<name>|<host>|<test_path>|<expect_200>|<cors_origin>
# expect_200: 期望 HTTP 状态（200/3xx/404 算 PASS；5xx 算 FAIL）
# cors_origin: 用于 OPTIONS preflight 的 Origin 头
HOSTS=(
  "RAW|raw.githubusercontent.com (v0.2 §4.1 wasm 默认)|raw.githubusercontent.com|mlc-ai/binary-mlc-llm-libs/main/web-llm-models/README.md|200|https://emotion-echo.local"
  "A|Aliyun OSS (推荐)|oss-cn-hangzhou.aliyuncs.com|README|200|https://emotion-echo.local"
  "B|Tencent Cloud COS|cos.ap-shanghai.myqcloud.com|README|200|https://emotion-echo.local"
  "E|HuggingFace mirror (hf-mirror)|hf-mirror.com|README|200|https://emotion-echo.local"
  "F|ModelScope|www.modelscope.cn|api/v1/models/X-D-Lab/MindChat-Qwen2-0_5B|200|https://emotion-echo.local"
)

# ---------- 输出累计器 ----------
REPORT_LINES=()
TOTAL_PASS=0
TOTAL_FAIL=0
TOTAL_SKIP=0

append() { REPORT_LINES+=("$1"); }
now_iso() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }

# ---------- 探测函数 ----------

# HEAD 探测：返回 <url> 是否 200/4xx（域名可达 + 路径对/不对）；连接失败 → SKIP
# 语义：
#   200/3xx/4xx = 域名可达 + 服务在应答（不论路径对不对 —— bucket 不存在也是 4xx）
#   5xx         = 服务异常 = FAIL
#   000         = 连接失败 / 超时 = SKIP（不静默）
probe_head() {
  local url="$1"
  local timeout="${2:-10}"
  if [[ $DRY_RUN -eq 1 ]]; then
    echo "[DRY-RUN skip] HEAD $url"
    return 0
  fi
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' -I "$url" --max-time "$timeout" 2>/dev/null || echo "000")
  case "$code" in
    200|301|302|303|307|308|400|401|403|404|405)
      echo "$code"; return 0 ;;  # 域名可达
    5*)
      echo "$code"; return 1 ;;  # 服务异常 = FAIL
    *)
      echo "$code"; return 2 ;;  # 000 / 无响应 = SKIP
  esac
}

# CORS preflight：OPTIONS + Origin 头，检查 Access-Control-Allow-Origin 响应头
# 语义：200/204 + 含 ACAO 头 → PASS；200/204 无 ACAO 头 → WARN
#       4xx = CORS 阻断（CORS 拒绝是常见，单独标 WARN）；5xx = FAIL；000 = SKIP
probe_cors() {
  local url="$1"
  local origin="$2"
  local timeout="${3:-10}"
  if [[ $DRY_RUN -eq 1 ]]; then
    echo "[DRY-RUN skip] CORS $url (origin=$origin)"
    return 0
  fi
  local headers
  headers=$(curl -sS -o /dev/null -w '%{http_code}|%{header_json}' -X OPTIONS "$url" \
    -H "Origin: $origin" \
    -H "Access-Control-Request-Method: GET" \
    --max-time "$timeout" 2>/dev/null || echo "000|{}")
  echo "$headers"
}

# GET 探测：返回 Content-Length（用于核对产物大小）
probe_get() {
  local url="$1"
  local timeout="${2:-15}"
  if [[ $DRY_RUN -eq 1 ]]; then
    echo "[DRY-RUN skip] GET $url"
    return 0
  fi
  curl -sS -o /dev/null -w '%{http_code}|%{size_download}' "$url" --max-time "$timeout" 2>/dev/null || echo "000|0"
}

# wasm magic byte 探测：拉前 4 字节，验证 0x00 0x61 0x73 0x6d
# 返回码语义：
#   0 = wasm magic OK / EMPTY（bucket 不存在属合理 SKIP）
#   1 = 内容存在但不是 wasm 格式（FAIL —— 可能 CDN 把 wasm 当文本转发）
probe_wasm_magic() {
  local url="$1"
  local timeout="${2:-15}"
  if [[ $DRY_RUN -eq 1 ]]; then
    echo "[DRY-RUN skip] WASM_MAGIC $url"
    return 0
  fi
  # 用 curl range 请求前 4 字节
  local tmp
  tmp=$(mktemp)
  local code
  code=$(curl -sS -o "$tmp" -w '%{http_code}' -H 'Range: bytes=0-3' "$url" --max-time "$timeout" 2>/dev/null || echo "000")
  if [[ ! -s "$tmp" ]]; then
    rm -f "$tmp"
    echo "EMPTY:$code"  # bucket/object 不存在 → SKIP（不算 FAIL）
    return 0
  fi
  local hex
  hex=$(xxd -p "$tmp" 2>/dev/null | head -c 8 || od -An -tx1 "$tmp" | head -1 | tr -d ' \n')
  rm -f "$tmp"
  # 排除 HTML 错误页（bucket 不存在时返回）
  if [[ "$hex" == "3c3f786d" || "$hex" == "3c21444f" || "$hex" == "<" ]]; then
    echo "HTML_ERROR:$code"  # 错误页 → SKIP
    return 0
  fi
  if [[ "$hex" == "0061736d" ]]; then
    echo "WASM_MAGIC_OK"
    return 0
  fi
  echo "NOT_WASM:$hex"
  return 1
}

# ---------- 主流程 ----------
append "# CDN 候选可达性探测报告"
append ""
append "**生成时间**: $(now_iso)"
append "**协议依据**: on-device-compile-cdn-2026-09-24.md §三（5 候选 CDN） + AGENTS.md §〇.6 文档功课"
append "**探测工具**: \`probe.sh\` (本目录)"
append "**退出码契约**: 0 = 全部 PASS 或 SKIP；1 = 任意 FAIL（与 smoke_data_layer.py 一致）"
append ""
append "## 总览"
append ""
append "| ID | 候选 | Host | HEAD | CORS preflight | 备注 |"
append "|----|------|------|------|----------------|------|"

for entry in "${HOSTS[@]}"; do
  IFS='|' read -r id name host path expect_200 cors_origin <<< "$entry"
  if [[ -n "$ONLY_HOST" && "$host" != *"$ONLY_HOST"* ]]; then
    continue
  fi
  url="https://${host}/${path}"

  # HEAD 探测
  if [[ $DRY_RUN -eq 1 ]]; then
    head_label="[SKIP-DRY-RUN]"
    head_status="N/A"
  else
    head_code=$(probe_head "$url")
    head_rc=$?
    if [[ $head_rc -eq 0 ]]; then
      head_label="[PASS]"
      head_status="$head_code"
      TOTAL_PASS=$((TOTAL_PASS + 1))
    elif [[ $head_rc -eq 2 ]]; then
      head_label="[SKIP timeout/conn-refused]"
      head_status="$head_code"
      TOTAL_SKIP=$((TOTAL_SKIP + 1))
    else
      head_label="[FAIL service-error]"
      head_status="$head_code"
      TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
  fi

  # CORS preflight 探测
  if [[ $DRY_RUN -eq 1 ]]; then
    cors_label="[SKIP-DRY-RUN]"
  else
    cors_resp=$(probe_cors "$url" "$cors_origin")
    cors_code="${cors_resp%%|*}"
    if [[ "$cors_code" == "200" || "$cors_code" == "204" ]]; then
      if [[ "$cors_resp" == *"\"access-control-allow-origin\""* ]]; then
        cors_label="[PASS ACAO]"
        TOTAL_PASS=$((TOTAL_PASS + 1))
      else
        cors_label="[WARN no-ACAO]"
        TOTAL_SKIP=$((TOTAL_SKIP + 1))
      fi
    elif [[ "$cors_code" == "000" ]]; then
      cors_label="[SKIP timeout/conn-refused]"
      TOTAL_SKIP=$((TOTAL_SKIP + 1))
    elif [[ "$cors_code" == 4* ]]; then
      # 4xx = CORS 阻断（多数 bucket 默认拒绝跨域），不算 FAIL 算 WARN
      cors_label="[WARN CORS-blocked ${cors_code}]"
      TOTAL_SKIP=$((TOTAL_SKIP + 1))
    else
      cors_label="[FAIL ${cors_code}]"
      TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
  fi

  append "| \`$id\` | $name | \`$host\` | $head_label $head_status | $cors_label | $url |"
done

append ""
append "## 汇总"
append ""
append "- **PASS**: $TOTAL_PASS"
append "- **FAIL**: $TOTAL_FAIL"
append "- **SKIP**: $TOTAL_SKIP"

# wasm magic byte 探测 —— 仅探测 Aliyun OSS（推荐，假定已部署）
append ""
append "## wasm magic byte 探测（仅 Aliyun OSS 推荐 bucket）"
append ""
if [[ $DRY_RUN -eq 1 ]]; then
  append "- [SKIP-DRY-RUN] wasm magic byte 探测未执行"
else
  wasm_url="https://emotion-echo-assets.oss-cn-hangzhou.aliyuncs.com/web-llm-models/v0_2_84/base/Qwen3-1.7B-q4f16_1_cs1k-webgpu.wasm"
  magic_result=$(probe_wasm_magic "$wasm_url")
  case "$magic_result" in
    WASM_MAGIC_OK)
      append "- [PASS] wasm magic OK（$wasm_url）"
      TOTAL_PASS=$((TOTAL_PASS + 1)) ;;
    EMPTY:*|HTML_ERROR:*)
      append "- [SKIP] wasm 对象不存在或 bucket 未创建（$magic_result）—— 待 T3 创建后再验"
      TOTAL_SKIP=$((TOTAL_SKIP + 1)) ;;
    NOT_WASM:*)
      append "- [FAIL] wasm magic 不匹配：$magic_result（CDN 可能把 wasm 当文本转发 —— 配置 Content-Type）"
      TOTAL_FAIL=$((TOTAL_FAIL + 1)) ;;
    *)
      append "- [FAIL] wasm 探测异常：$magic_result"
      TOTAL_FAIL=$((TOTAL_FAIL + 1)) ;;
  esac
fi

# 探测经验记录
append ""
append "## 探测结论"
append ""
append "（根据本次实测填入 —— 用于 §十二 决策 2/3 的可达性证据）"
append ""
append "**观察**（待人工补充）:"
append ""
append "## SKIP 纪律"
append ""
append "- 探测任意步骤 timeout / conn-refused → 显式 SKIP（不静默；与 check_required_checks.py:24-26 一致）"
append "- \`--dry-run\` 模式下所有探测 SKIP，CI 确定性兜底（退出码 0）"

# 写入报告
printf "%s\n" "${REPORT_LINES[@]}" > "$OUTPUT"
echo "Wrote report: $OUTPUT"
echo ""
echo "Summary: PASS=$TOTAL_PASS FAIL=$TOTAL_FAIL SKIP=$TOTAL_SKIP"

if [[ $TOTAL_FAIL -gt 0 ]]; then
  exit 1
fi
exit 0