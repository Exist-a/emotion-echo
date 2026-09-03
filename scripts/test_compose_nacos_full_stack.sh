#!/usr/bin/env bash
# test_compose_nacos_full_stack.sh · deploy/docker-compose.apps.yml Nacos 全栈契约
#
# Stage 36-FU Bug 10 follow-up:
#   报告 §九 写 "Bug 10 NACOS_ENABLED=true + 各 svc 配置中心 bootstrap"
#   但 §T5 又说 "实际 NACOS_ENABLED=false, env 直读"。两个状态不一致。
#
# 本契约测试 RED→GREEN: 断言 compose 文件里所有声明接入 Nacos 的 svc,
# 都正确注入了 NACOS_ENABLED=true + NACOS_ADDR=emotion-echo-nacos:8848,
# 且 nacos 服务在 infra compose 中存在并先于 apps 启动。
#
# 这是 yaml-level 契约;运行时端到端 (实际注册到 Nacos) 留给生产环境
# 验证 (跟 Stage 36-B5 决策一致: 镜像构建留给生产网络跑)。
#
# Usage: bash scripts/test_compose_nacos_full_stack.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APPS_YML="$REPO_ROOT/deploy/docker-compose.apps.yml"
INFRA_YML="$REPO_ROOT/deploy/docker-compose.infra.yml"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "  ok: $*"; }

echo "compose-nacos-full-stack contract test"
echo "  APPS_YML = $APPS_YML"
echo "  INFRA_YML = $INFRA_YML"

[ -f "$APPS_YML" ]  || fail "apps compose not found"
[ -f "$INFRA_YML" ] || fail "infra compose not found"

python - "$APPS_YML" "$INFRA_YML" <<'PY' || exit 1
import re, sys, pathlib

apps_path = pathlib.Path(sys.argv[1])
infra_path = pathlib.Path(sys.argv[2])

apps_text = re.sub(r'^\s*#.*$', '', apps_path.read_text(encoding="utf-8"), flags=re.MULTILINE)
infra_text = re.sub(r'^\s*#.*$', '', infra_path.read_text(encoding="utf-8"), flags=re.MULTILINE)

# 1. infra compose must have nacos service on 8848
print("\n§1 infra compose has nacos service on 8848")
if not re.search(r"^  nacos:\s*$", infra_text, re.MULTILINE):
    print("FAIL: infra compose missing 'nacos:' service"); sys.exit(1)
print("  ok: infra compose has 'nacos:' service")

if not re.search(r"8848:8848|emotion-echo-nacos:8848", infra_text):
    print("FAIL: infra compose does not expose nacos 8848"); sys.exit(1)
print("  ok: infra compose exposes nacos 8848")

# 2. apps compose — enumerate services (skip top-level keys + nested networks/volumes)
print("\n§2 apps compose services must inject NACOS_ENABLED=true + NACOS_ADDR")

# Find each service block. A service is "  <name>:" with no leading whitespace under
# the service name (i.e. the indented body must use at least 4 spaces).
service_blocks = []
current = None
current_body = []
for line in apps_text.splitlines():
    m = re.match(r"^  ([a-zA-Z][a-zA-Z0-9_-]*):\s*$", line)
    if m:
        if current is not None:
            service_blocks.append((current, "\n".join(current_body)))
        current = m.group(1)
        current_body = []
        continue
    if current is None:
        continue
    current_body.append(line)
if current is not None:
    service_blocks.append((current, "\n".join(current_body)))

# Skip non-service top-level entries
top_level = {"x-logging-env", "networks", "volumes", "services"}
service_blocks = [(name, body) for name, body in service_blocks
                  if name not in top_level and not name.startswith("x-")]

report = []
errors = []
for name, body in service_blocks:
    ne = re.search(r"^\s{6}NACOS_ENABLED:\s*\"?(true|1)\"?", body, re.MULTILINE)
    na = re.search(r"^\s{6}NACOS_ADDR:\s*\"?([^\s\"]+)\"?", body, re.MULTILINE)
    has_dep = bool(re.search(r"^\s+- nacos\s*$", body, re.MULTILINE)) or "      nacos:" in body
    report.append((name, bool(ne), na.group(1) if na else None, has_dep))
    if ne and not na:
        errors.append(f"{name}: NACOS_ENABLED=true but NACOS_ADDR missing")
    if na and not ne:
        errors.append(f"{name}: NACOS_ADDR set but NACOS_ENABLED missing")

# All services that touch Nacos env must be fully wired
for name, enabled, addr, dep in report:
    if not (enabled or addr):
        continue
    print(f"  {name}: NACOS_ENABLED={enabled} NACOS_ADDR={addr} depends_on_nacos={dep}")
    if not enabled:
        errors.append(f"{name}: NACOS_ENABLED missing")
    if not addr:
        errors.append(f"{name}: NACOS_ADDR missing")
    if not dep:
        errors.append(f"{name}: depends_on nacos missing")

if errors:
    print("\nFAIL: incomplete Nacos env in apps compose:")
    for e in errors:
        print(f"  - {e}")
    sys.exit(1)

# Count: must have at least 6 services (5 Go + ai-svc + llm + bff; subset)
nacos_services = [r for r in report if r[1]]
if len(nacos_services) < 6:
    print(f"\nFAIL: expected >= 6 services with NACOS_ENABLED, got {len(nacos_services)}")
    sys.exit(1)
print(f"\n  ok: {len(nacos_services)} services wired to Nacos (>= 6)")

# 3. Verify the runtime side: shared pkg/configcenter and shared pkg/discovery have tests
print("\n§3 shared/pkg/configcenter + shared/pkg/discovery have config-bootstrap tests")
configcenter_test = pathlib.Path("emotion-echo-shared/pkg/configcenter/nacos_config_test.go")
discovery_test = pathlib.Path("emotion-echo-shared/pkg/discovery/nacos_register_test.go")
if not configcenter_test.is_file():
    print(f"FAIL: missing {configcenter_test}"); sys.exit(1)
if not discovery_test.is_file():
    print(f"FAIL: missing {discovery_test}"); sys.exit(1)
print(f"  ok: {configcenter_test} present")
print(f"  ok: {discovery_test} present")

# 4. Per-svc nacos_boot_test.go presence (5 Go svc + ai-svc + bff; llm-service is Python)
print("\n§4 per-svc nacos_boot_test.go coverage")
required = [
    "emotion-echo-user-svc/nacos_boot_test.go",
    "emotion-echo-chat-svc/nacos_boot_test.go",
    "emotion-echo-analytics-svc/nacos_boot_test.go",
    "emotion-echo-assessment-svc/nacos_boot_test.go",
    "emotion-echo-ai-svc/nacos_boot_test.go",
    "emotion-echo-web-bff/nacos_boot_test.go",
]
for path in required:
    p = pathlib.Path(path)
    if not p.is_file():
        print(f"FAIL: missing {path}"); sys.exit(1)
    print(f"  ok: {path}")

# 5. llm-service Python side: nacos_bootstrap test present
print("\n§5 emotion-llm-service Nacos bootstrap test (Python)")
llm_test = pathlib.Path("emotion-llm-service/tests/unit/test_nacos_bootstrap.py")
if not llm_test.is_file():
    print(f"FAIL: missing {llm_test}"); sys.exit(1)
print(f"  ok: {llm_test} present")

print("\nPASS: compose-nacos-full-stack contract green")
PY
echo ""
echo "all checks passed."