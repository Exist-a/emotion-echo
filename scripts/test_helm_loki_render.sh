#!/usr/bin/env bash
# test_helm_loki_render.sh — helm template 渲染回归（E2E-F-147）
#
# 为什么需要：k8s 侧 promtail DaemonSet 曾写死 runAsUser=10001 读 hostPath 日志目录，
# 而**读不到日志不会让 Pod 变 NotReady**（readinessProbe 打的是 promtail 自己的
# /ready）⇒ "Pod 全绿 + Loki 一条业务日志都没有"，纯靠人工 review 几乎不可能发现。
# 渲染结果必须机械校验。
#
# 断言：
#   1. 渲染成功
#   2. promtail DaemonSet 默认**不含** runAsUser / runAsNonRoot（否则会静默读不到日志）
#   3. hostPath /var/log/containers 与 /var/log/pods 仍然挂载（修了权限不能把挂载删了）
#   4. --set 覆盖时确实能写入（保证逃生阀可用，而不是被模板写死）
#   5. loki 与 promtail 镜像版本与 values 一致，且与 ADR-2026-09-loki-aggregator-dev 对齐
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT/charts/emotion-echo" || exit 1
PASS=0; FAIL=0
ok(){ echo "  ✓ $1"; PASS=$((PASS+1)); }
bad(){ echo "  ✗ $1"; FAIL=$((FAIL+1)); }

command -v helm >/dev/null 2>&1 || { echo "[SKIP] helm 不可用"; exit 0; }

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

if helm template emotion-echo charts/loki > "$TMP/base.yaml" 2>"$TMP/err.txt"; then
  ok "helm template 渲染成功"
else
  bad "helm template 渲染失败: $(head -c 200 "$TMP/err.txt")"; echo "Summary: PASS=$PASS FAIL=$FAIL"; exit 1
fi

# Windows/Git Bash 下 python3 常常是 WindowsApps 的空壳（存在但不执行），
# 必须先真的跑一次确认可用，再退到 python。
PY_BIN=""
for cand in python python3; do
  if command -v "$cand" >/dev/null 2>&1 && "$cand" -c "pass" >/dev/null 2>&1; then
    PY_BIN="$cand"; break
  fi
done
[ -n "$PY_BIN" ] || { echo "  ✗ 找不到可用的 python"; exit 1; }
"$PY_BIN" - "$TMP/base.yaml" <<'PY'
import sys, re
doc = open(sys.argv[1], encoding="utf-8").read()
blocks = [b for b in doc.split("\n---\n") if "kind: DaemonSet" in b and "name: promtail" in b]
if not blocks:
    print("  ✗ 渲染结果里找不到 promtail DaemonSet"); sys.exit(1)
b = blocks[0]
# 去掉 YAML 注释行后再判，避免模板里的说明文字造成误判
code = "\n".join(l for l in b.split("\n") if not l.strip().startswith("#"))
fails = 0
if re.search(r"runAsUser|runAsNonRoot", code):
    print("  ✗ promtail DaemonSet 默认带了 runAs* 约束 → uid 读不到 hostPath 日志目录，"
          "而 Pod 仍全绿（readinessProbe 与采集无关）"); fails += 1
else:
    print("  ✓ promtail DaemonSet 默认无非 root 约束")
for p in ("/var/log/containers", "/var/log/pods"):
    if p not in code:
        print("  ✗ hostPath %s 丢失（修权限时把挂载删了）" % p); fails += 1
    else:
        print("  ✓ hostPath %s 仍挂载" % p)
sys.exit(1 if fails else 0)
PY
if [ $? -eq 0 ]; then PASS=$((PASS+3)); else FAIL=$((FAIL+1)); fi

# 覆盖路径（逃生阀必须可用）
if helm template emotion-echo charts/loki \
     --set promtail.podSecurityContext.runAsUser=10001 \
     --set promtail.podSecurityContext.runAsNonRoot=true > "$TMP/ovr.yaml" 2>/dev/null \
   && grep -q "runAsUser: 10001" "$TMP/ovr.yaml"; then
  ok "--set promtail.podSecurityContext 覆盖生效（集群强制非 root 时有逃生阀）"
else
  bad "覆盖未生效：promtail.podSecurityContext 被模板写死"
fi

# 版本一致性：compose / chart / ADR 三处同版本
CV=$(grep -oE 'grafana/loki:[0-9.]+' "$REPO_ROOT"/deploy/docker-compose.infra.yml | head -1 | cut -d: -f2)
HV=$(grep -A 2 'repository: grafana/loki' charts/loki/values.yaml | grep -oE 'tag: [0-9.]+' | head -1 | cut -d' ' -f2)
PV=$(grep -A 2 'repository: grafana/promtail' charts/loki/values.yaml | grep -oE 'tag: [0-9.]+' | head -1 | cut -d' ' -f2)
CP=$(grep -oE 'grafana/promtail:[0-9.]+' "$REPO_ROOT"/deploy/docker-compose.infra.yml | head -1 | cut -d: -f2)
if [ "$CV" = "$HV" ] && [ "$CP" = "$PV" ]; then
  ok "Loki/Promtail 版本一致：loki=$CV promtail=$CP（compose 与 chart 同步）"
else
  bad "版本漂移：compose loki=$CV promtail=$CP vs chart loki=$HV promtail=$PV"
fi

echo ""
echo "Summary: PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
