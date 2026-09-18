#!/usr/bin/env bash
# scripts/check_secrets.sh —— 明文密钥扫描（E2E-F-69 的防复发机制 / E2E-03 D3）
#
# 为什么需要（AGENTS.md §四红线"key 永远不进版本库"）：
#   2026-09-18 实测发现 APISIX 管理员密钥与 BFF JWT 密钥的**真实可用默认值**被内联在
#   deploy/apisix/config.yaml、seed.sh、docker-compose.apps.yml 等处，而仓库是 public
#   ⇒ 任何人读出密钥 + 可达 9180 端口即可完全接管网关配置。
#   红线只写在文档里、没有机器检查 ⇒ 必然复发（本项目已反复验证）。故补此扫描器。
#
# 三条规则（都为"高置信度、低噪声"设计）：
#   规则 1  已知泄露字面量（历史实际泄露过的值）—— 命中即 FAIL
#   规则 2  公开可识别的令牌格式（GitHub PAT / OpenAI / AWS AK / GitLab PAT）
#   规则 3  把 20+ 位十六进制/base64 直接赋给"密钥语义"的变量或字段
#
# 噪声控制（每条都由实测假阳性驱动，见行内注释）：
#   - 跳过注释行（# / // / /* / *）：允许文档叙述历史值
#   - 跳过"值其实是函数调用"的行（如 `const token = getClientAccessToken()`）：
#     规则 3 针对**字面量**，函数调用不是字面量
#   - 豁免 env 兜底（${...}）、占位符（local-only / change-me / placeholder / example /
#     dummy / your-）、测试夹具标记（test / fake / sample / min / padding / xxx / invalid）
#     —— 假阳性会让人开始忽略门禁，比没有门禁更糟（anti-patterns AP-11）
#
# 扫描范围（含曾漏掉的盲区，均由 E2E-F-69 复查发现）：
#   - deploy/（含 *.sh / *.py / *.js / *.yaml）
#   - docs/env-templates/（**配置模板**，不是散文；原先 docs/ 整体排除把它一起漏了）
#   - 各 Go svc、emotion-llm-service、scripts、.github
#   - emotion-echo-web/app + emotion-echo-web/e2e（覆盖 *.ts / *.vue / *.js）
#   有意不扫：docs/*.md（散文，允许叙述历史值）、charts/（决策 23 已冻结且不部署）、
#             .git、node_modules
#
# 用法：bash scripts/check_secrets.sh
#   SECRET_SCAN_TARGETS 可覆盖扫描目标（空格分隔目录/文件），供负向测试用 fixture
# 退出码：0 = 未发现明文密钥；1 = 命中

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

DEFAULT_TARGETS=(
  deploy docs/env-templates
  emotion-echo-web-bff emotion-echo-shared emotion-echo-user-svc emotion-echo-chat-svc
  emotion-echo-ai-svc emotion-echo-analytics-svc emotion-echo-assessment-svc
  emotion-llm-service scripts .github
  emotion-echo-web/app emotion-echo-web/e2e
)
if [ -n "${SECRET_SCAN_TARGETS:-}" ]; then
  # shellcheck disable=SC2206
  TARGETS=(${SECRET_SCAN_TARGETS})
else
  TARGETS=("${DEFAULT_TARGETS[@]}")
fi

# 规则 1：历史实际泄露过的字面量（本文件须列出它们，故不参加扫描）
KNOWN_LEAKED=(
  "WhZEPlrGviCSXlKFfALZlQWinluoGAbj"
)
# 规则 2：令牌格式
TOKEN_RE='(ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{15,}|sk-[A-Za-z0-9]{24,}|AKIA[0-9A-Z]{16})'
# 规则 3：密钥语义变量 = 长字面量。不加 PCRE 内联标志——bash 的 [[ =~ ]] 是 POSIX ERE，
# 大小写不敏感交给 grep -i（曾因写 (?i) 导致本规则从未生效，由负向测试用例 ③ 抓出）。
SECRETISH_RE='(secret|passwd|password|api[_-]?key|access[_-]?key|private[_-]?key|token)["'"'"']?[[:space:]]*[:=][[:space:]]*["'"'"']?[A-Za-z0-9/+_-]{20,}'
# 豁免标记（grep -i 承担大小写不敏感）
EXEMPT_RE='(\$\{|local-only|change-me|changeme|placeholder|example|dummy|your-|<.*>|test|fake|sample|min|padding|xxx|invalid)'
# 注释行前缀（# / // / /* / *）
COMMENT_RE=':[0-9]+:[[:space:]]*(#|//|/\*|\*)'
# "值其实是函数调用"过滤
CALL_RE='[:=][[:space:]]*[A-Za-z_][A-Za-z0-9_.]*\('

collect_files() {
  for t in "${TARGETS[@]}"; do
    [ -e "$t" ] || continue
    if [ -f "$t" ]; then
      echo "$t"
    else
      find "$t" -type f 2>/dev/null \
        | grep -Ev '/(node_modules|\.git)/' \
        | grep -E '\.(yml|yaml|sh|py|go|json|env|example|tpl|conf|js|ts|vue)$' \
        | grep -v 'check_secrets.sh$'
    fi
  done
}

LIST="$(mktemp)"
trap 'rm -f "$LIST"' EXIT
collect_files > "$LIST"

echo "=== 明文密钥扫描 ==="
if [ ! -s "$LIST" ]; then
  echo "GREEN: 无待扫描文件"
  exit 0
fi

lit_args=()
for lit in "${KNOWN_LEAKED[@]}"; do lit_args+=(-e "$lit"); done

raw="$(
  {
    xargs -a "$LIST" grep -nHF "${lit_args[@]}" 2>/dev/null
    xargs -a "$LIST" grep -nHE "$TOKEN_RE" 2>/dev/null
    xargs -a "$LIST" grep -niE "$SECRETISH_RE" 2>/dev/null
  } \
    | sort -u \
    | grep -vE "$COMMENT_RE" \
    | grep -vE "$CALL_RE" \
    | grep -vE "$EXEMPT_RE" \
    || true
)"

if [ -n "$raw" ]; then
  printf '%s\n' "$raw" | sed 's/^/  /'
  echo ""
  echo "RED: 命中 $(printf '%s\n' "$raw" | grep -c .) 处疑似明文密钥。"
  echo "     修法：把真值移到 deploy/.env.local（gitignored，用 --env-file 注入），"
  echo "           仓库里只留显式占位符；若确为误报，请改进本脚本的豁免规则而非直接忽略。"
  exit 1
fi

echo "GREEN: 未发现明文密钥（扫描 $(grep -c . "$LIST") 个文件）"
exit 0
