"""Lane O · 端侧化阶段一 · 云端基线跑分（v0.3 §C.1 任务 5）。

注入真实 model_fn（云端 gRPC / DeepSeek HTTP / 录播回放）跑既有
`scripts/on-device-golden/golden_set.jsonl`，产出 baseline_report.md。

与 `scripts/on-device-golden/` 的边界：
- golden/ 是纯模型（无 IO 无网络，AGENTS §一.1.3）
- baseline/ 是实战层（gRPC 客户端 + mock fallback + 录播落盘），pytest 用 fake stub 隔离网络
"""