---
purpose: 系统演进历史
status: Round 1 占位 · Round 3 全部迁入
---

# 系统演进历史（Stage 0 → 36+）

> 每个 Stage 是一次完整、独立、可回滚的演进单元。
> 每份 stage 文档包含目标、commit 清单、验证报告、ADR 引用。

## 阅读建议

- **新人**：先看 Stage 35（生产加固）→ Stage 30（BFF 上线）→ Stage 0~5（基础）
- **找某次变更**：直接按 stage 编号定位
- **想了解某个决策**：`architecture/adr/` + 对应 stage 文档

## 索引

Round 3 完成后此索引将自动生成（按 stage 编号）。

| 阶段 | 主题 | 文档数 |
|------|------|-------|
| Stage 0~9 | 单体 → 微服务演进 | 10 |
| Stage 10~19 | gRPC 化 | 10 |
| Stage 20~25 | 容器化 + AI 接入 | ~15 |
| Stage 26~30 | BFF + Kafka + LLM | ~25 |
| Stage 31~36 | Nacos + APISIX + 多模态 + 生产加固 | ~30 |
