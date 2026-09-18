# Scripts 目录

本目录包含项目自动化脚本。

## 门禁脚本（R-03）

| 脚本 | 用途 | 依据 |
|------|------|------|
| `e2e_stage_audit.py` | E2E 阶段收口审计器（11 条断言） | RUNBOOK §13 |
| `check_tdd_gate.sh` | TDD 门禁：生产代码改动须有测试 | AGENTS.md §0 |
| `check_adr_gate.sh` | ADR 门禁：架构改动须有 ADR | anti-patterns AP-08 |
| `check_orphan_outputs.sh` | 孤儿产出物检测 | anti-patterns AP-10 |
| `check_docker_digests.sh` | Dockerfile digest 校验 | D-07 |
| `lint_env_vars.sh` | 环境变量 lint | ADR-18 |

## 测试脚本

| 脚本 | 用途 |
|------|------|
| `test_migrate_checksum.sh` | migrate.sh checksum 负向测试 |
| `test_migrations_no_service_order.sh` | migration 服务顺序独立性检查 |
| `test_migrations_contract.sh` | migration 契约检查 |

## 业务脚本

| 脚本 | 用途 |
|------|------|
| `smoke_data_layer.py` | 数据层 smoke 测试 |
| `seed-demo-account.sh` | 演示账号种子脚本 |
| `cleanup-demo-account.sh` | 演示账号清理脚本 |
| `healthcheck_smoke.sh` | 健康检查 smoke |

## CI 相关

| 脚本 | 用途 |
|------|------|
| `sync_docker_digests.sh` | 同步 Docker digest（需 registry 可达） |
| `check_minio_health.sh` | MinIO 健康检查 |
