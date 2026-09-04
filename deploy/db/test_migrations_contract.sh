#!/usr/bin/env bash
# test_migrations_contract.sh — 数据库迁移自动应用的契约测试
#
# 背景（2026-09-04 实测）：仓库里 13 个 */migrations/*.sql 从来没有任何机制会执行，
# 全靠人工 `docker exec ... psql < 文件`，且无处记录跑没跑过。实际后果是 dev 库
# 一直缺列/缺视图/缺角色：
#   POST /conversations/{id}/messages → 500
#   column "client_msg_id" of relation "messages" does not exist
# 补一个之后 smoke 反而从"看起来 10/10"暴露到 1/10。
#
# 为什么不能靠 initdb.d 解决：Postgres 的 docker-entrypoint-initdb.d 只在数据卷
# 为空时执行一次。挂进去对已有环境无效，之后新增的迁移对所有已存在环境也无效。
# initdb.d 只能表达"初始状态"，表达不了"持续演进"。故用一次性 migrate 容器。
#
# 跑：bash deploy/db/test_migrations_contract.sh

set -eu

PG_CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-emotion_echo}"
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MIGRATE_SH="$REPO_ROOT/deploy/db/migrate.sh"

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[PASS] $*"; }

psql_q() {
  docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -tAc "$1" 2>&1
}

echo "=== 契约 1: migrate.sh 存在且可执行 ==="
[ -f "$MIGRATE_SH" ] || fail "缺 deploy/db/migrate.sh —— 迁移仍无自动应用机制"
pass "migrate.sh 存在"

echo
echo "=== 契约 2: 每个带 migrations/ 的服务都必须登记在 migrate.sh 的执行顺序里 ==="
# 这条是本测试的核心：防止"新增了迁移文件但没人登记"重演今天的状态。
# 用发现式断言而非硬编码清单——新服务加 migrations/ 就会被抓出来。
missing=""
for dir in "$REPO_ROOT"/*/migrations; do
  [ -d "$dir" ] || continue
  svc=$(basename "$(dirname "$dir")")
  # legacy/ 下的历史迁移不参与
  case "$svc" in legacy) continue ;; esac
  n=$(find "$dir" -maxdepth 1 -name '*.sql' | wc -l | tr -d ' ')
  [ "$n" -gt 0 ] || continue
  if ! grep -q "$svc" "$MIGRATE_SH"; then
    missing="$missing $svc($n个)"
  else
    pass "$svc 已登记（$n 个迁移）"
  fi
done
[ -z "$missing" ] || fail "这些服务有迁移但没登记进 migrate.sh：$missing"

echo
echo "=== 契约 3: 关键 schema 对象存在（缺任一都会让业务 500 / smoke 变红） ==="
# 每条都对应本轮实际踩到的故障
col=$(psql_q "SELECT 1 FROM information_schema.columns WHERE table_schema='emotion_echo_chat' AND table_name='messages' AND column_name='client_msg_id'")
[ "$col" = "1" ] || fail "messages.client_msg_id 列不存在 —— 发消息会 500（chat 002 未应用）"
pass "messages.client_msg_id 存在"

col=$(psql_q "SELECT 1 FROM information_schema.columns WHERE table_schema='emotion_echo_analytics' AND table_name='user_behavior_events' AND column_name='event_id'")
[ "$col" = "1" ] || fail "user_behavior_events.event_id 列不存在 —— Kafka consumer 写入失败（analytics 006 未应用）"
pass "user_behavior_events.event_id 存在"

role=$(psql_q "SELECT 1 FROM pg_roles WHERE rolname='analytics_reader'")
[ "$role" = "1" ] || fail "analytics_reader 角色不存在 —— smoke §契约 3 必红（analytics 004 未应用）"
pass "analytics_reader 角色存在"

for v in "emotion_echo_chat.msg_summary_v" "emotion_echo_ai.daily_emotion_v" "emotion_echo_assessment.assessment_v"; do
  sch=${v%%.*}; nm=${v##*.}
  got=$(psql_q "SELECT 1 FROM information_schema.views WHERE table_schema='$sch' AND table_name='$nm'")
  [ "$got" = "1" ] || fail "视图 $v 不存在 —— /reports/daily 会 500（analytics 001 未应用）"
  pass "视图 $v 存在"
done

got=$(psql_q "SELECT 1 FROM information_schema.views WHERE table_schema='emotion_echo_ai' AND table_name='daily_emotion_by_modality_v'")
[ "$got" = "1" ] || fail "视图 daily_emotion_by_modality_v 不存在（ai 005 未应用；它依赖 ai 002/003 建的表）"
pass "视图 daily_emotion_by_modality_v 存在"

echo
echo "=== 契约 4: 全部迁移可重复执行（migrate 容器每次 up 都会跑） ==="
# 抓 analytics/004 那类无 IF NOT EXISTS 守卫的迁移：首次成功、第二次 CREATE ROLE 报错。
out=$(bash "$MIGRATE_SH" 2>&1) || fail "重复执行迁移失败（非幂等）：$(echo "$out" | tail -5)"
pass "全部迁移重复执行成功（幂等）"

echo
echo "=== 契约 5: initdb 链完整执行（03 种子 / 04 视图未被中途掐断） ==="
# 2026-09-04 实测：01 与 02 重复定义 emotion_analysis 且列不一致，导致
# 02:117 CREATE UNIQUE INDEX (event_id) 报错，**中断整条 initdb 链**——
# 03-seed-default-users.sql / 04-create-views.sql 完全未执行，users 表为空、
# 登录 401、smoke 根本跑不起来。这类"前一个脚本失败静默掐断后续"最难排查，
# 故单独立契约。
n=$(psql_q "SELECT COUNT(*) FROM emotion_echo_user.users WHERE username='echo'")
[ "$n" = "1" ] || fail "种子用户 echo 不存在 —— initdb 链可能在 03 之前就中断了（登录会 401）"
pass "种子用户 echo 存在（03 已执行）"

err=$(docker logs "$PG_CONTAINER" 2>&1 | grep -c "docker-entrypoint-initdb.d.*ERROR" || true)
[ "$err" = "0" ] || fail "postgres initdb 日志里有 $err 条 ERROR —— 链被中断，后续脚本未执行"
pass "initdb 日志无 ERROR"

echo
echo "迁移契约全部 PASS"
