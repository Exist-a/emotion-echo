# ADR-2026-10: XTTS 推理卸载线程池（并发解串行）

## Status

✅ **Accepted**（2026-10-03，E2E-28 C4 / D-43 落地；决策 39 同日登记）

## Context

**账本 F-136**（E2E-17 时代留账，E2E-28 转挂执行）记："XTTS 单 worker 串行 → 长文请求排队"，修法指向"多 worker（多 :8003 端口 + 负载均衡）"。

E2E-28 #11 量化实测（2026-10-03，双并发 `/api/v1/tts/phonemes` 经网关）：

- 两请求同时提交 → 完成时刻 `[12.22s, 23.27s]`，后者 ≈ 两者之和 ⇒ **严格串行，排队等待 ≈ 12.2s**

server.py 回读定位**深层根因**（账本原记根因修正）：

```python
@app.post("/tts_with_phonemes")
async def tts_with_phonemes(req):        # ← async def
    outputs = tts_model.synthesize(...)  # ← 同步阻塞 13~30s，直调在事件循环里
```

**async handler 在事件循环里直调阻塞推理 ⇒ 整个事件循环锁死 ⇒ 第二个请求连被处理都要等第一个完成**。`/tts`、`/tts_stream`（async 生成器逐块阻塞）同根因。所以"uvicorn 单 worker"只是表象——**即使多进程/多 worker，每个进程内的事件循环仍会锁死自己的并发**（只是池子变多）。

## Decision

**修法 = 线程池卸载（`emotion-echo-models/XTTS/synth_pool.py`），否决进程级多 worker。**

1. `run_synth(fn, *args, workers=None)`：`ThreadPoolExecutor` 池（容量 env `XTTS_SYNTH_WORKERS`，默认 **2**）+ `loop.run_in_executor`；异常透传。
2. `/tts` 与 `/tts_with_phonemes` 的 `tts_model.synthesize(...)` → `await run_synth(...)`。
3. `stream_audio_generator`：`async def` → **同步 `def`** —— Starlette `StreamingResponse` 对同步迭代器自动 `iterate_in_threadpool`，块生成在池线程推进。
4. compose xtts 段显式声明 `XTTS_SYNTH_WORKERS: ${XTTS_SYNTH_WORKERS:-2}`（调优面可见化；设 `1` 退化串行为回滚开关）。

### 为什么否决进程级多 worker（备选）

| 维度 | 线程池（选定） | 进程级多 worker（否决） |
|------|--------------|------------------------|
| 内存 | **零增量**（共享模型） | 每 worker ~2.5GiB 模型常驻；`.wslconfig memory=8GB` + 19 容器稳态 ~6G ⇒ **扩容即击穿**（2026-09-22 Docker 冻结三连实案，见 memory 记录） |
| 真并行性 | torch CPU 算子释放 GIL，推理可真并行 | 真并行（多进程），但每个进程内事件循环阻塞问题**依旧存在**，仍需本 ADR 的卸载 —— 两层都要做等于双倍改动 |
| 改动面 | server.py 3 处 + 1 新模块 | uvicorn 启动方式重构（手动 `main()` + 自定义信号处理不兼容 `uvicorn.run(workers=N)`）+ compose 第二实例 + BFF 负载均衡 |
| 回滚 | env 置 1 即回退 | 删容器/改 BFF |

### 为什么线程池能真并行（不是 GIL 一刀切）

PyTorch CPU 算子（matmul/conv 等）在计算期间**释放 GIL**；两个推理的重活分别在两个池线程里执行，可同时吃多核。`torchaudio.save`/base64 编码段持 GIL 但占比小（<5%）。已知余量风险：两个推理各默认 torch 8 线程在 8 核上限下超订 —— 与 F-132 的超订同型但方向相反（那次是 2 核配 8 线程），执行期复测若单请求延迟明显劣化，再调 `torch.set_num_threads`（不在本 ADR 预先加码）。

## Consequences

**正面**：

- 并发 TTS 请求不再排队（运行时验收：#11 同法复测两请求应重叠）
- 零内存增量，不触碰 8GB WSL 红线
- `XTTS_SYNTH_WORKERS=1` 保留原串行语义作回滚开关

**负面 / 留账**：

- torch 线程超订可能在双满载时让单请求略变慢（执行期实测定性；劣化明显则调 torch 线程数）
- 模型推理的线程安全性依赖 PyTorch 推理只读权重的常规保证（无官方并发 inference 契约；运行时复测若见崩溃/脏结果 → 回退 workers=1 并记账）
- **账本 F-136 的根因描述已被本 ADR 修正**（单 worker 表象 → 事件循环阻塞真根因），收口时回填 ledger

## 实证

| 项 | 修前 | 修后 |
|---|---|---|
| 两并发 /tts/phonemes（#11 同法） | 串行 [12.22s, 23.27s]，排队 ≈12.2s | 运行时复测见 E2E-28 report #11 |
| XTTS 单元测试 | 73 passed | **75 passed**（synth_pool 并发/容量/透传/静态接线 8 断言 + GREEN 配套 2 断言） |
| 内存 | 2.54GiB | 不变（零增量） |

## 调研依据

- [x] `server.py` 三 handler 回读（`/tts` :168-176 / `/tts_with_phonemes` :302-310 直调；`stream_audio_generator` :207 原 async）
- [x] E2E-28 plan #11 双并发实测时间戳（`baseline/tts_queue_concurrency2.json`）
- [x] `.wslconfig` 8GB + 19 容器稳态 ~6G + 2026-09-22 冻结三连（memory: Docker 数据盘 VHDX 管理）
- [x] `pcm_chunk_shape` 抽模块先例（Stage 26-T §5.2.24，同型"可测性抽取"路径）
- [x] XTTS 既有测试约定：不 import server.py（torch 2GB 加载链）⇒ 静态文本契约 + 独立模块单测
- [x] Starlette `StreamingResponse` 对同步迭代器走 `iterate_in_threadpool`（官方行为，静态契约钉 `def stream_audio_generator`）

## 关联

- E2E 轨决策：**D-43**（E2E-28 M2，用户拍板"双端点+多 worker 全做"——目标=消除排队，实现路径按本 ADR 根因修正）
- 架构决策：**决策 39**（`docs/architecture/decisions.md`）
- 账本：E2E-F-136（根因描述回填后闭环）、E2E-F-132（CPU 限额守卫，本 ADR 不动 cpus=8.0）
- 测试：`emotion-echo-models/XTTS/tests/unit/test_synth_pool.py`（RED `a312db7` → GREEN `b37cf44`）
