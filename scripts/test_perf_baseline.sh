#!/usr/bin/env bash
# E2E-28 C1 · perf_baseline.py 的契约守卫（plan §3 C1，先红后绿）。
#
# 断言三件事（对应 plan 测试点 #1）：
#   1. percentile 已知样本断言 —— 官方线性插值方法（docs.python.org statistics
#      "simpler alternative" 公式：i=(len-1)*p/100，线性插值），
#      [1..100] → p50=50.5 / p95=95.05。数字错了，所有基线都不可信。
#   2. 负向对照 —— 指向未监听端口必须**非零退出**且 stderr 有显式 ERROR。
#      防"端口不通也静默 PASS"（AP-01 同型：产出物存在 ≠ 行为正确）。
#   3. 正向通路 —— 对本地起的真实 HTTP 服务跑 N=5，rc=0 且 stdout JSON 里
#      summary.n==5、含 p50/p95。防脚本"只会报错不会测量"。
#
# 路径纪律：全程 `cd REPO_ROOT` + 相对路径调 python —— Git Bash 的 /d/、/tmp
# MSYS 路径 Windows python 解析不了（E2E-28 计划期实测 /tmp/login.json 打不开
# 同型坑），相对路径 + cwd 在两边都成立。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT" || exit 2
PORT=18999
fail=0
pass=0
SERVER_PID=""

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null
}
trap cleanup EXIT

echo "== E2E-28 perf_baseline 契约守卫 =="

if [ ! -f scripts/perf_baseline.py ]; then
  echo "FAIL 被测脚本不存在: scripts/perf_baseline.py（C1 RED 状态）"
  exit 1
fi

echo
echo "--- 1) percentile 已知样本（官方线性插值）---"
if python - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location(
    "perf_baseline", "scripts/perf_baseline.py")
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
data = list(range(1, 101))
cases = [(m.percentile(data, 50), 50.5), (m.percentile(data, 95), 95.05),
         (m.percentile(data, 0), 1.0), (m.percentile(data, 100), 100.0)]
for got, want in cases:
    assert abs(got - want) < 0.01, f"percentile mismatch: got {got}, want {want}"
# 小样本（N=5）也不许崩 —— plan #2 N>=50 / #6 N>=5 均可能
small = [0.3, 0.1, 0.4, 0.2, 0.5]
assert 0.1 <= m.percentile(small, 50) <= 0.5, "small-sample p50 out of range"
print("percentile cases OK")
PY
then
  echo "PASS percentile：p50=50.5/p95=95.05/端点/小样本全中"
  pass=$((pass + 1))
else
  echo "FAIL percentile：已知样本断言未过"
  fail=$((fail + 1))
fi

echo
echo "--- 2) 负向对照：未监听端口必须非零退出 + 显式 ERROR ---"
NEG_URL="http://127.0.0.1:9/"
if python -c "import socket; s=socket.socket(); r=s.connect_ex(('127.0.0.1',9)); s.close(); exit(0 if r!=0 else 1)"; then
  neg_err="$( { python scripts/perf_baseline.py --url "$NEG_URL" --n 1 >/dev/null; } 2>&1 )"
  neg_rc=$?
  if [ "$neg_rc" -ne 0 ] && echo "$neg_err" | grep -q "ERROR"; then
    echo "PASS 负向：rc=$neg_rc 且 stderr 含显式 ERROR"
    pass=$((pass + 1))
  else
    echo "FAIL 负向：rc=$neg_rc（期望非零）stderr=[$neg_err]"
    fail=$((fail + 1))
  fi
else
  echo "WARN 负向：本机 127.0.0.1:9 已被监听，跳过负向用例（正向/分位数仍须过）"
fi

echo
echo "--- 3) 正向通路：对真实本地 HTTP 服务测 N=5 ---"
python -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1 &
SERVER_PID=$!
# 等服务就绪（最多 5s；http.server 默认 cwd = REPO_ROOT，仅回目录页无副作用）
ready=0
for _ in $(seq 1 25); do
  if python -c "import socket; s=socket.socket(); r=s.connect_ex(('127.0.0.1',$PORT)); s.close(); exit(0 if r==0 else 1)"; then
    ready=1
    break
  fi
  sleep 0.2
done
if [ "$ready" -ne 1 ]; then
  echo "FAIL 正向：本地 http.server 未就绪（环境问题，非被测对象）"
  fail=$((fail + 1))
else
  # stdout 捕获进变量、JSON 经 stdin 喂 python —— mktemp 的 /tmp 路径
  # Windows python 打不开（MSYS 路径坑），stdin 无路径问题
  err_file="$(mktemp)"
  out="$(python scripts/perf_baseline.py --url "http://127.0.0.1:$PORT/" --n 5 2>"$err_file")"
  rc=$?
  if [ "$rc" -eq 0 ] && printf '%s' "$out" | python -c "
import json, sys
d = json.load(sys.stdin)
s = d['summary']
assert s['n'] == 5, f\"n={s['n']} != 5\"
assert 'p50' in s and 'p95' in s, 'missing percentiles'
assert d['errors'] == [], f\"errors={d['errors']}\"
assert s['min'] >= 0 and s['p50'] <= s['p95'], 'inconsistent summary'
"
  then
    echo "PASS 正向：N=5 rc=0 且 JSON summary.n=5 + p50/p95 存在"
    pass=$((pass + 1))
  else
    echo "FAIL 正向：rc=$rc out=[$(printf '%s' "$out" | head -c 300)] err=[$(head -c 300 "$err_file")]"
    fail=$((fail + 1))
  fi
  rm -f "$err_file"
fi

echo
echo "--- 4) SSE 模式：--n 多次运行 + 逐块时间戳 + burst_ratio ---"
# 本地起 SSE 服务器：6 个 data 块、间隔 0.6s（> 0.5s burst 窗口 ⇒ burst_ratio 应低）。
# 契约（plan #6/#7）：--n 2 → summary.n==2（多次运行）；每轮 ttfb 有值、
# data_chunks==6、burst_ratio < 0.8（渐进流不是整段缓冲）。
SSE_PORT=18998
cat > sse_test_server.py <<'PYEOF'
import sys, time
from http.server import BaseHTTPRequestHandler, HTTPServer

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        for i in range(6):
            self.wfile.write(f'data: {{"choices":[{{"delta":{{"content":"c{i}"}}}}]}}\n\n'.encode())
            self.wfile.flush()
            time.sleep(0.6)
    def log_message(self, *a):
        pass

HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PYEOF
python sse_test_server.py "$SSE_PORT" >/dev/null 2>&1 &
SERVER_PID=$!
sse_ready=0
for _ in $(seq 1 25); do
  if python -c "import socket; s=socket.socket(); r=s.connect_ex(('127.0.0.1',$SSE_PORT)); s.close(); exit(0 if r==0 else 1)"; then
    sse_ready=1; break
  fi
  sleep 0.2
done
if [ "$sse_ready" -ne 1 ]; then
  echo "FAIL SSE：本地 SSE server 未就绪"
  fail=$((fail + 1))
else
  err_file2="$(mktemp)"
  sse_out="$(python scripts/perf_baseline.py --mode sse --url "http://127.0.0.1:$SSE_PORT/sse" --n 2 --timeout 30 2>"$err_file2")"
  sse_rc=$?
  if [ "$sse_rc" -eq 0 ] && printf '%s' "$sse_out" | python -c "
import json, sys
d = json.load(sys.stdin)
assert d['mode'] == 'sse'
s = d['summary']
assert s['n'] == 2, f\"summary.n={s['n']} != 2 (--n 未生效)\"
assert s['p50'] is not None and s['p95'] is not None, 'missing ttfb percentiles'
for r in d['runs']:
    assert r['ttfb_ms'] is not None, 'missing ttfb'
    assert r['data_chunks'] == 6, f\"chunks={r['data_chunks']} != 6\"
    assert r['burst_ratio'] is not None and r['burst_ratio'] < 0.8, (
        f\"burst_ratio={r['burst_ratio']} (0.6s 间隔流应远小于 0.8)\")
assert d['errors'] == [], f\"errors={d['errors']}\"
"
  then
    echo "PASS SSE：n=2 + ttfb 分位数 + chunks=6 + burst_ratio<0.8"
    pass=$((pass + 1))
  else
    echo "FAIL SSE：rc=$sse_rc out=[$(printf '%s' "$sse_out" | head -c 300)] err=[$(head -c 300 "$err_file2")]"
    fail=$((fail + 1))
  fi
  rm -f "$err_file2"
fi
rm -f sse_test_server.py
SERVER_PID=""

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：perf_baseline 契约未满足"
  exit 1
fi

echo "GREEN：percentile 官方方法 / 负向非零退出 / 正向 N=5 测量 三契约全过"
