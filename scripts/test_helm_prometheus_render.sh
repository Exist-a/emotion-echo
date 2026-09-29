#!/usr/bin/env bash
# test_helm_prometheus_render.sh — helm template 渲染回归（E2E-22 · 测试点 #14/#15）
#
# 为什么需要：k8s 侧 prometheus chart 曾声明
#     rule_files: /etc/prometheus/rules/*.yml
# 而该 ConfigMap 的 data **只有 prometheus.yml 一个 key**、deployment 也只挂了
# config(/etc/prometheus) 与 data(/prometheus) 两个 volume
#   ⇒ 规则目录为空 ⇒ rule_files 命中**空 glob**
#   ⇒ Prometheus **不报任何错**，静默加载 0 条规则、0 条告警。
# 这与 E2E-F-147（promtail 无权限读日志但 Pod 全绿）同型："配置声明了但没接上"，
# 纯靠人工 review 发现不了。渲染结果必须机械校验。
#
# ⚠️ 覆盖边界：本机无 k8s 集群，本脚本只验证**渲染产物**，
#    不验证集群内运行时（生产部署时需实测一次）。
#
# 断言：
#   1. helm template 渲染成功
#   2. prometheus-rules ConfigMap 存在，且内联了全部告警规则文件
#   3. deployment 挂载了 /etc/prometheus/rules 且引用 prometheus-rules
#   4. rule_files 声明与实际挂载路径一致（防"声明改了、挂载没改"）
#   5. ConfigMap 里的告警名与 deploy/prometheus/rules/*.yml 双向一致（防双份漂移）
#   6. 负向对照：把挂载改掉后本脚本必须失败
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHART_DIR="$REPO_ROOT/charts/emotion-echo"
PASS=0; FAIL=0
ok(){ echo "  ✓ $1"; PASS=$((PASS+1)); }
bad(){ echo "  ✗ $1"; FAIL=$((FAIL+1)); }

command -v helm >/dev/null 2>&1 || { echo "[SKIP] helm 不可用"; exit 0; }

# Windows/Git Bash 下 python3 可能是 WindowsApps 空壳（存在但不执行）
PY_BIN=""
for cand in python python3; do
  if command -v "$cand" >/dev/null 2>&1 && "$cand" -c "pass" >/dev/null 2>&1; then
    PY_BIN="$cand"; break
  fi
done
[ -n "$PY_BIN" ] || { echo "  ✗ 找不到可用的 python"; exit 1; }

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# ---------------------------------------------------------------------------
# 1. 渲染
# ---------------------------------------------------------------------------
cd "$CHART_DIR" || exit 1
if helm template emotion-echo charts/prometheus > "$TMP/base.yaml" 2>"$TMP/err.txt"; then
  ok "helm template charts/prometheus 渲染成功"
else
  bad "helm template 渲染失败: $(head -c 300 "$TMP/err.txt")"
  echo "Summary: PASS=$PASS FAIL=$FAIL"; exit 1
fi

# ---------------------------------------------------------------------------
# 2~5. 渲染产物断言
# ---------------------------------------------------------------------------
"$PY_BIN" - "$TMP/base.yaml" "$REPO_ROOT" <<'PY'
import sys, os, re, glob

doc = open(sys.argv[1], encoding="utf-8").read()
repo = sys.argv[2]
fails = []
def ok(m):  print("  ✓ " + m)
def bad(m): fails.append(m); print("  ✗ " + m)

blocks = doc.split("\n---\n")

# --- 2. prometheus-rules ConfigMap ---
cm = [b for b in blocks if "kind: ConfigMap" in b and "name: prometheus-rules" in b]
if not cm:
    bad("渲染结果里找不到 prometheus-rules ConfigMap —— rule_files 会命中空 glob")
    cm_body = ""
else:
    ok("prometheus-rules ConfigMap 存在")
    cm_body = cm[0]

expected_files = sorted(
    os.path.basename(f) for f in glob.glob(os.path.join(repo, "deploy/prometheus/rules/*.yml"))
)
missing_keys = [f for f in expected_files if f + ":" not in cm_body]
if missing_keys:
    bad(f"ConfigMap 缺规则文件 key: {missing_keys}（dev 侧有 {len(expected_files)} 个规则文件）")
else:
    ok(f"ConfigMap 内联全部 {len(expected_files)} 个规则文件: {expected_files}")

# --- 3. deployment 挂载 ---
dep = [b for b in blocks if "kind: Deployment" in b and "prometheus" in b]
if not dep:
    bad("渲染结果里找不到 prometheus Deployment")
    dep_body = ""
else:
    dep_body = dep[0]
    code = "\n".join(l for l in dep_body.split("\n") if not l.strip().startswith("#"))

    if "mountPath: /etc/prometheus/rules" in code:
        ok("deployment 挂载 /etc/prometheus/rules")
    else:
        bad("deployment 未挂载 /etc/prometheus/rules —— rule_files 指向空目录，规则永不加载")

    if re.search(r"name:\s*rules\s*\n\s*configMap:\s*\n\s*name:\s*prometheus-rules", code):
        ok("rules volume 指向 prometheus-rules ConfigMap")
    else:
        bad("rules volume 未指向 prometheus-rules ConfigMap")

    # --- 4. rule_files 声明与挂载路径一致 ---
    # 注意: 声明值是带 glob 的文件 glob（/etc/prometheus/rules/*.yml），
    # 而 volume 挂载的是其父目录 —— 比对时必须剥掉 glob 部分，
    # 否则会永远判"脱节"（这正是本脚本第一版的 bug）。
    cfg = [b for b in blocks if "kind: ConfigMap" in b and "name: prometheus-config" in b]
    if cfg:
        m = re.search(r"rule_files:\s*\n\s*-\s*(\S+)", cfg[0])
        declared = m.group(1) if m else None
        if declared is None:
            bad("prometheus-config 未声明 rule_files")
        else:
            # /etc/prometheus/rules/*.yml -> /etc/prometheus/rules
            declared_dir = re.sub(r"/\*.*$", "", declared)
            if "mountPath: " + declared_dir in code:
                ok(f"rule_files 声明 {declared} 的挂载目录 {declared_dir} 已接线")
            else:
                bad(f"rule_files 声明 {declared}（目录 {declared_dir}），但无对应挂载 —— 声明与接线脱节（静默失效）")

# --- 5. 告警名双向一致（dev 规则文件 vs chart 内联副本）---
dev_alerts = set()
for f in glob.glob(os.path.join(repo, "deploy/prometheus/rules/*.yml")):
    for m in re.finditer(r"^\s*-?\s*alert:\s*(\S+)", open(f, encoding="utf-8").read(), re.M):
        dev_alerts.add(m.group(1))
chart_alerts = set(re.findall(r"^\s*- alert:\s*(\S+)", cm_body, re.M))

miss = sorted(dev_alerts - chart_alerts)
extra = sorted(chart_alerts - dev_alerts)
if not dev_alerts:
    bad("dev 侧未解析到任何告警规则 —— 解析器或规则目录失效")
if miss:
    bad(f"chart 缺告警: {miss} —— k8s 环境这些告警不会响")
if extra:
    bad(f"chart 多出告警: {extra} —— 与 dev 不一致")
if not miss and not extra and dev_alerts:
    ok(f"chart 内联告警与 dev 侧 {len(dev_alerts)} 条完全一致: {sorted(dev_alerts)}")

sys.exit(1 if fails else 0)
PY
RC=$?
[ $RC -eq 0 ] && PASS=$((PASS+6)) || FAIL=$((FAIL+1))

# ---------------------------------------------------------------------------
# 6. 负向对照：拿掉挂载后本脚本必须失败
#    （没有这条，"渲染断言通过"可能只是正则太松，等于没测）
#
# ⚠️ 递归守卫：子进程必须以绝对路径调用，且带 E2E22_NEG=1 跳过自身的负向对照。
#    否则子进程会再次进入本段并无限递归；而若误用相对路径 $0，本段会在
#    `cd "$CHART_DIR"` 之后找不到脚本而直接失败 —— 表现为"负向对照通过了"，
#    实际是脚本没跑起来（验证工具自身 bug 会把失败误报成通过）。
# ---------------------------------------------------------------------------
SELF_ABS="$REPO_ROOT/scripts/test_helm_prometheus_render.sh"

if [ "${E2E22_NEG:-}" = "1" ]; then
  # 负向子进程：只跑正向断言，到此为止
  echo "Summary: PASS=$PASS FAIL=$FAIL"
  [ "$FAIL" -eq 0 ] || exit 1
  exit 0
fi

DEPLOY_TPL="$CHART_DIR/charts/prometheus/templates/deployment.yaml"
BACKUP="$TMP/deployment.yaml.bak"
cp "$DEPLOY_TPL" "$BACKUP"
trap 'cp "$BACKUP" "$DEPLOY_TPL" 2>/dev/null; rm -rf "$TMP"' EXIT

# 删除 rules volumeMount 与 volume 两处
"$PY_BIN" - "$DEPLOY_TPL" <<'PY'
import sys, re
p = sys.argv[1]
s = open(p, encoding="utf-8").read()
s = re.sub(r"\n\s*# E2E-22: rule_files 声明[\s\S]*?mountPath: /etc/prometheus/rules", "", s)
s = re.sub(r"\n\s*- name: rules\n\s*configMap:\n\s*name: prometheus-rules", "", s)
open(p, "w", encoding="utf-8").write(s)
PY

if grep -q "mountPath: /etc/prometheus/rules" "$DEPLOY_TPL"; then
  bad "负向对照无法执行：rules 挂载未被移除，注入的破坏未生效"
else
  if E2E22_NEG=1 bash "$SELF_ABS" > "$TMP/neg.txt" 2>&1; then
    bad "负向对照失败：移除 rules 挂载后本脚本仍返回 0 —— 断言没有约束力"
  else
    if grep -q "未挂载 /etc/prometheus/rules\|声明与接线脱节\|rules volume 未指向" "$TMP/neg.txt"; then
      ok "负向对照通过：移除 rules 挂载后断言如期变红"
    else
      bad "负向对照：子进程确实失败了，但失败原因不是 rules 挂载断言（可能被其它错误掩盖）：$(grep -m3 '✗' "$TMP/neg.txt" | head -3 | tr '\n' ' ')"
    fi
  fi
fi
cp "$BACKUP" "$DEPLOY_TPL"

# ---------------------------------------------------------------------------
echo "Summary: PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
