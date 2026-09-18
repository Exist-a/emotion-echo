#!/usr/bin/env bash
# scripts/check_secrets.sh —— 明文密钥扫描（E2E-F-69 的防复发机制 / E2E-03 D3）
#
# 为什么需要（AGENTS.md §四红线）：
#   "key 永远不进版本库"。但 2026-09-18 实测发现：APISIX 管理员密钥与 BFF JWT
#   密钥的**真实可用默认值**被内联在 deploy/apisix/config.yaml、seed.sh、
#   docker-compose.apps.yml 里，而仓库是 public ⇒ 任何人读出密钥、配合可达的
#   9180 端口即可完全接管网关配置。红线只写在文档里、没有机器检查 ⇒ 必然复发。
#
# 三条规则（都为"高置信度、低噪声"设计）：
#   规则 1：已知泄露字面量（历史实际泄露过的值）—— 命中即 FAIL。
#   规则 2：公开可识别的令牌格式（GitHub PAT / OpenAI key / AWS AK / GitLab PAT）。
#   规则 3：把 20+ 位十六进制/base64 直接赋给"密钥语义"的变量或字段。
#           刻意排除：含 ${...} 的行（env 兜底默认值）与含 placeholder 类
#           标记的行（local-only / change-me / example / dummy / placeholder），
#           否则会把"占位符"误报成密钥（本脚本自身与仓库现状都不应被误报）。
#
# 有意不扫的目录：.git、node_modules、docs/（文档叙述可提到历史值）、
#   charts/（Helm 已冻结不部署，见 decisions.md 决策 23）——
#   剩余范围见 RUNBOOK §13.3 的说明。
#
# 用法：bash scripts/check_secrets.sh
# 退出码：0 = 未发现明文密钥；1 = 命中

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# SCAN_TARGETS 可覆盖（供负向测试用临时 fixture）
DEFAULT_TARGETS=(deploy emotion-echo-web-bff emotion-echo-shared emotion-echo-user-svc
                 emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-analytics-svc
                 emotion-echo-assessment-svc emotion-llm-service scripts .github)
if [ -n "${SECRET_SCAN_TARGETS:-}" ]; then
  # shellcheck disable=SC2206
  TARGETS=(${SECRET_SCAN_TARGETS})
else
  TARGETS=("${DEFAULT_TARGETS[@]}")
fi

# 规则 1：历史实际泄露过的字面量
KNOWN_LEAKED=(
  "WhZEPlrGviCSXlKFfALZlQWinluoGAbj"
)
# 规则 2：令牌格式
TOKEN_RE='(ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{15,}|sk-[A-Za-z0-9]{24,}|AKIA[0-9A-Z]{16})'
# 规则 3：密钥语义变量 = 长字面量
# 注意：不加 (?i) —— bash 的 [[ =~ ]] 是 POSIX ERE，不支持 PCRE 内联标志；
# 大小写不敏感由 grep -iE 承担（曾因 (?i) 导致本规则从未生效，负向测试用例 ③ 抓出）。
SECRETISH_RE='(secret|passwd|password|api[_-]?key|access[_-]?key|private[_-]?key|token)["'"'"']?[[:space:]]*[:=][[:space:]]*["'"'"']?[A-Za-z0-9/+_-]{20,}'
# 规则 3 的豁免标记（占位符）与 env 兜底
EXEMPT_RE='(\$\{|local-only|change-me|changeme|placeholder|example|dummy|your-|<.*>)'

SCAN_EXT=(*.yml *.yaml *.sh *.py *.go *.json *.env *.example *.tf *.tpl *.conf)

collect_files() {
  for t in "${TARGETS[@]}"; do
    [ -e "$t" ] || continue
    if [ -f "$t" ]; then
      printf '%s
' "$t"
    else
      find "$t" -type f \( -name '*.yml' -o -name '*.yaml' -o -name '*.sh' -o -name '*.py'         -o -name '*.go' -o -name '*.json' -o -name '*.example' -o -name '*.tpl' -o -name '*.conf' \)         -not -path '*/node_modules/*' -not -path '*/.git/*' -not -name 'check_secrets.sh' 2>/dev/null
    fi
  done
}

# --- 扫描核心：单次 grep 取代逐行 bash 循环 ---
# 原实现逐行做 3 次 [[ =~ ]]（704 文件），CI 上 5 分钟未完成，会拖慢每个 PR。
# 改为把文件列表交给 grep 一次扫完（实测 5 秒级），再统一过滤注释行与豁免标记。
LIST="$(mktemp)"
collect_files > "$LIST"
if [ ! -s "$LIST" ]; then
  echo "GREEN: 无待扫描文件"; rm -f "$LIST"; exit 0
fi

lit_args=()
for lit in "${KNOWN_LEAKED[@]}"; do lit_args+=(-e "$lit"); done

raw="$( {
    xargs -a "$LIST" grep -nHF "${lit_args[@]}" 2>/dev/null
    xargs -a "$LIST" grep -nHE "$TOKEN_RE" 2>/dev/null
    xargs -a "$LIST" grep -niE "$SECRETISH_RE" 2>/dev/null
  } | sort -u     | grep -vE ':[0-9]+:[[:space:]]*#'     | grep -vE "$EXEMPT_RE"     | grep -v 'check_secrets.sh:' || true )"
rm -f "$LIST"

hits=0
report=""
if [ -n "$raw" ]; then
  report="$(printf '%s
' "$raw" | sed 's/^/  /')"$'
'
  hits="$(printf '%s
' "$raw" | grep -c .)"
fi

if [ "$hits" -gt 0 ]; then
  echo "$report" | head -30
  echo ""
  echo "RED: 命中 $hits 处疑似明文密钥。"
  echo "     修法：把真值移到 deploy/.env.local（gitignored，用 --env-file 注入），"
  echo "           仓库里只留显式占位符；若确为误报，请改进本脚本的豁免规则而非直接忽略。"
  exit 1
fi

echo "GREEN: 未发现明文密钥（扫描 $(collect_files | grep -c .) 个文件）"
exit 0
