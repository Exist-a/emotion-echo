#!/usr/bin/env bash
# test_web_and_healthcheck_contracts.sh · Bug 9 + G1 契约测试
#
# Stage 36-FU Bug 9 + G1 follow-up:
#   Bug 9 (partial): commit 723e18b 修了 Emotion-Echo-Web/Dockerfile,
#     但 deploy/docker-compose.apps.yml 把 emotion-echo-web 设成
#     `profiles: ["never"]` 显式禁用, 根目录 docker-compose.yml 的
#     frontend 服务仍指向 Emotion-Echo-Web/Dockerfile (旧 / 没 ARG)
#   G1 healthcheck: ${SKYWALKING_OAP_ADDR:-...} 占位符没默认值, 4 svc
#     启动时报 "unhealthy" 但 /health 200 (依赖硬编码缺失)
#
# RED→GREEN:
#   - assert emotion-echo-web service 不在 profiles: ["never"]
#   - assert 根目录 docker-compose.yml frontend service Dockerfile 接受
#     NPM_REGISTRY build arg
#   - assert 每个 go svc 的 healthcheck 用了 `${SKYWALKING_OAP_ADDR:-...}`
#     默认值 fallback (而不是裸 ${SKYWALKING_OAP_ADDR})
#
# Usage: bash scripts/test_web_and_healthcheck_contracts.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ROOT_COMPOSE="$REPO_ROOT/docker-compose.yml"
APPS_COMPOSE="$REPO_ROOT/deploy/docker-compose.apps.yml"
WEB_DOCKERFILE="$REPO_ROOT/Emotion-Echo-Web/Dockerfile"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "  ok: $*"; }

echo "web + healthcheck contract test"

[ -f "$ROOT_COMPOSE" ]    || fail "root docker-compose.yml not found"
[ -f "$APPS_COMPOSE" ]    || fail "apps compose not found"
[ -f "$WEB_DOCKERFILE" ]  || fail "web Dockerfile not found"

# ---------------------------------------------------------------
# Bug 9a: emotion-echo-web must not be in profiles: ["never"]
# ---------------------------------------------------------------
echo ""
echo "§1 emotion-echo-web service in apps compose must NOT be in profiles: never"

python - "$APPS_COMPOSE" <<'PY' || exit 1
import re, sys, pathlib
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
# Strip comments for parser
text_clean = re.sub(r'^\s*#.*$', '', text, flags=re.MULTILINE)

# Locate emotion-echo-web service block
m = re.search(
    r"^  emotion-echo-web:\s*\n((?:^    .*\n?|^\s*\n)*)",
    text_clean, re.MULTILINE,
)
if not m:
    print("FAIL: emotion-echo-web service block not found in apps compose")
    sys.exit(1)

block = m.group(1)
if re.search(r"profiles:\s*\[\s*\"never\"\s*\]", block):
    print("FAIL: emotion-echo-web still has profiles: [\"never\"] — Bug 9 partial regression")
    sys.exit(1)
print("  ok: emotion-echo-web removed from profiles: never")
PY

# ---------------------------------------------------------------
# Bug 9b: web Dockerfile must accept NPM_REGISTRY ARG
# ---------------------------------------------------------------
echo ""
echo "§2 Emotion-Echo-Web/Dockerfile must accept NPM_REGISTRY build arg"

python - "$WEB_DOCKERFILE" <<'PY' || exit 1
import re, sys, pathlib
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
if not re.search(r"ARG\s+NPM_REGISTRY", text):
    print(f"FAIL: {sys.argv[1]} missing `ARG NPM_REGISTRY` — Bug 9 Dockerfile regression")
    sys.exit(1)
if "registry.npmmirror.com" in text and "ARG NPM_REGISTRY" not in text.split("ARG NPM_REGISTRY", 1)[1] if "ARG NPM_REGISTRY" in text else True:
    # If the npmmirror URL appears AFTER the ARG, that's still bugged
    pass
if "registry.npmmirror.com" in text:
    # Find position relative to ARG
    arg_pos = text.find("ARG NPM_REGISTRY")
    mirror_pos = text.find("registry.npmmirror.com")
    if mirror_pos > arg_pos:
        # After the ARG is fine if there's no usage, but check it's only in comments
        # Get the substring after the npmmirror mention
        sub = text[mirror_pos:]
        # If it appears in a real instruction (not comment), fail
        for line in sub.splitlines():
            stripped = line.lstrip()
            if stripped.startswith("#"):
                continue
            if "registry.npmmirror.com" in line:
                print(f"FAIL: npmmirror.com hard-coded outside ARG: {line!r}")
                sys.exit(1)
print("  ok: NPM_REGISTRY ARG present, npmmirror is not hard-coded outside ARG")
PY

# ---------------------------------------------------------------
# Bug 9c: root docker-compose.yml frontend should either be removed
#         (legacy) or use the fixed Dockerfile. Acceptable: pointing
#         at a Dockerfile that declares NPM_REGISTRY.
# ---------------------------------------------------------------
echo ""
echo "§3 root docker-compose.yml frontend section"

python - "$ROOT_COMPOSE" "$WEB_DOCKERFILE" <<'PY' || exit 1
import re, sys, pathlib

root_text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
web_text = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8")

# Either remove frontend entirely, or keep it but ensure the build context
# uses the fixed Dockerfile (with NPM_REGISTRY ARG)
m = re.search(
    r"^  frontend:\s*\n((?:^    .*\n?|^\s*\n)*)",
    root_text, re.MULTILINE,
)

if m is None:
    print("  ok: root docker-compose.yml has no legacy frontend service (clean)")
else:
    block = m.group(1)
    # If frontend is still defined, check it uses Dockerfile with ARG NPM_REGISTRY
    uses_dockerfile = "dockerfile: Dockerfile" in block or "dockerfile:" in block
    if uses_dockerfile:
        if "ARG NPM_REGISTRY" not in web_text:
            print("FAIL: frontend service in root compose uses Emotion-Echo-Web/Dockerfile "
                  "but the Dockerfile is missing `ARG NPM_REGISTRY`")
            sys.exit(1)
        print("  ok: root compose frontend uses Dockerfile with NPM_REGISTRY ARG")
    else:
        print("  ok: root compose frontend uses an image reference, no build context")
PY

# ---------------------------------------------------------------
# G1: healthcheck path. Each Go svc's healthcheck uses wget against
# localhost:port/health — the failure path was reported as
# "7 containers unhealthy but /health 200", meaning docker thinks
# they're unhealthy even when /health is OK. The fix is documented in
# commit 2699e89 (yaml healthcheck field). Verify that all 4 Go svc
# healthcheck blocks do declare `healthcheck:` with `start_period`
# (so docker doesn't restart-loop during model preload).
# ---------------------------------------------------------------
echo ""
echo "§4 G1: all Go svc + ai-svc + bff declare healthcheck with start_period"

python - "$APPS_COMPOSE" <<'PY' || exit 1
import re, sys, pathlib
text = re.sub(r'^\s*#.*$', '', pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"), flags=re.MULTILINE)

required_services = [
    "emotion-echo-user-svc",
    "emotion-echo-chat-svc",
    "emotion-echo-analytics-svc",
    "emotion-echo-assessment-svc",
    "emotion-echo-ai-svc",
    "emotion-echo-web-bff",
]

errors = []
for svc in required_services:
    m = re.search(
        rf"^  {re.escape(svc)}:\s*\n((?:^    .*\n?|^\s*\n)*)",
        text, re.MULTILINE,
    )
    if not m:
        errors.append(f"{svc}: service block not found")
        continue
    block = m.group(1)
    if "healthcheck:" not in block:
        errors.append(f"{svc}: healthcheck: missing")
        continue
    if "start_period:" not in block:
        errors.append(f"{svc}: healthcheck.start_period missing (G1 regression)")
        continue
    print(f"  ok: {svc} has healthcheck + start_period")

if errors:
    print("\nFAIL: G1 healthcheck contracts not met:")
    for e in errors:
        print(f"  - {e}")
    sys.exit(1)
PY

# ---------------------------------------------------------------
# G1b: env injection — every Go svc that uses SkyWalking should
# have SKYWALKING_OAP_ADDR set to the actual container DNS, not
# a bash default placeholder that go-zero can't expand.
# ---------------------------------------------------------------
echo ""
echo "§5 G1b: SKYWALKING_OAP_ADDR is set to container DNS (no bare \${VAR})"

python - "$APPS_COMPOSE" <<'PY' || exit 1
import re, sys, pathlib
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")

# Look for any line with `${SKYWALKING_OAP_ADDR}` (no `:-default` fallback)
# inside environment blocks. These are the bug: go-zero won't expand them.
bad = []
for m in re.finditer(r"^\s{6}SKYWALKING_OAP_ADDR:\s*(\S+)\s*$", text, re.MULTILINE):
    val = m.group(1).strip()
    if val.startswith("${") and ":-" not in val:
        bad.append((m.lineno, val))

if bad:
    print("FAIL: SKYWALKING_OAP_ADDR has unexpanded placeholders:")
    for ln, v in bad:
        print(f"  line {ln}: {v}")
    sys.exit(1)

# Verify the actual values are concrete (not placeholders)
required = ["emotion-echo-sw-oap:11800"]
concrete = re.findall(r"^\s{6}SKYWALKING_OAP_ADDR:\s*(\S+)\s*$", text, re.MULTILINE)
concrete = [c for c in concrete if c != "$${SKYWALKING_OAP_ADDR}"]
if not any("emotion-echo-sw-oap" in c for c in concrete):
    print("FAIL: no concrete SKYWALKING_OAP_ADDR pointing at emotion-echo-sw-oap:11800")
    sys.exit(1)

print(f"  ok: {len(concrete)} SKYWALKING_OAP_ADDR entries use concrete container DNS")
PY

echo ""
echo "PASS: web + healthcheck contracts green"