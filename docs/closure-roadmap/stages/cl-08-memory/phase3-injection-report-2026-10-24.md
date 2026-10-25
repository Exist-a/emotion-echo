---
status: done
type: phase-report
stage: CL-08c
phase: 3
created: 2026-10-24
related:
  - plan.md（§3 Phase 3 / §2 T-7~T-12）
  - phase2-extract-worker-report-2026-10-24.md
  - evidence/phase3-clf70-fix-2026-10-24.txt
  - evidence/phase3-t5-episode-runtime-2026-10-24.txt
  - evidence/iab-cl08c-memory-page-2026-10-24.png
  - evidence/iab-cl08c-cross-session-recall-2026-10-24.png
---

# CL-08c Phase 3 · BFF 注入段报告（2026-10-24）

> **落地载体**：阶段 PR [#277](https://github.com/Exist-a/emotion-echo/pull/277)；登记 PR #278 → `79844481`。
> **环境**：dev 栈 18 容器 healthy（`ai-svc`/`web-bff`/`web` = 本轨 `:cl08c` 镜像，其余公共 tag）；栈锁 `lane-cl-08c`。

---

## §1 范围校正（第三轮同型：开工先实读已落物）

plan.md §3 Phase 3 的主体——`segmentKindMemory` / `buildMemorySegment()` / Redis 用户级摘要读取 /
空段零回归 / ≤600 token 预算（`clampMemorySegment` 1200 rune）/ 段序 `base→agent→personality→memory→emotion→guard` /
两处调用点传参——**已由 CL-08a 全部落地**（`prompt_assembly.go` + `memory_source.go` + `downstream/memory_summary.go`）。
⇒ 本轮不重复，只做两件事：**修 CL-F-70① 的真缺陷** + **补运行时证据（T-5 A/B + IAB）**。

## §2 CL-F-70① 修复（本轮唯一的生产行为改动）

**RED（真缺陷）**：黑洞 Redis（连接通、永不回包）下 `LatestMemorySummary` 实测 **3.00s** 才返回——
go-redis 池化连接初始化不吃 ctx deadline，拖到默认 ReadTimeout 3s；违反代码自注释的 **300ms 契约**
（记忆是增强不是依赖，不能把首字延迟拖死）。

**GREEN**：goroutine + select 到点必返（实测 **0.30s**）；慢查询后台自行超时回收（有界，仅病态路径）。

**补齐的测试**（`downstream/memory_summary_test.go` + `prompt_assembly_memory_test.go` 追加）：
命中（key 与 ai-svc 写侧同源，抓跨服务 key 漂移）/ `redis.Nil → ("", nil)` / 黑洞 300ms / nil 装配 /
handler 全路径真实源（miniredis）/ **T-8 两处调用点钉**（字面量计数，防单路径漂移）。

## §3 测试点结论

| # | 测试点 | 判定 | 证据 |
|---|---|---|---|
| T-7 | 无记忆时逐字零回归 | **PASS（沿用 CL-08a）** | `TestPromptAssembly_NoMemory_ByteIdenticalToGolden` |
| T-8 | 段序不变量 + 两处调用点 | **PASS（本轮补钉）** | 段序钉沿用；`TestAIStreamHandler_BothAssembleSites_LoadMemorySummary` 本轮补 |
| T-9 | 记忆段内容正确 | **PASS（本轮升级为运行时）** | IAB：新会话答出三个跨会话事实（见 §4） |
| T-10 | 关键路径不查图 | **PASS（架构级）** | 注入路径只读 Redis（`MemorySummarySource`），BFF 无图/DB 依赖 |
| T-11 | 降级 | **PASS（本轮补强）** | fake source 四分支（CL-08a）+ 真实源全分支（本轮，含 300ms 黑洞实测） |
| T-12 | token 预算 | **PASS（沿用 CL-08a）** | 写侧 600 token + 读侧 1200 rune 兜底 |
| T-5 | 抽取失败不阻塞主链路（Phase 2 遗留的运行时半场） | **PASS（本轮补运行时）** | 三段 A/B，见 §4 |
| Phase 2 补证 | episode/entity 运行时落库 | **PASS（本轮顺带取证）** | 基线消息 episode 原文落库 + 恢复段新消息 episode 补齐 |

## §4 运行时证据（2026-10-24，dev 栈）

**T-5 三段 A/B**（⚠️ 原引 `phase3-t5-episode-runtime-2026-10-24.txt` 有误——该文件当时只含 IAB 段，A/B 输出因产物落盘位置错误未随 #277 进库，第二方核对 P1 抓出；**重放原始输出见 `cl08c-t5-rerun-2026-10-25.txt`**，base/down/up 全 PASS）：
- **base**：发消息（msg 1045）→ run=`done|1` + **episode 原文完整落库** + entity 1 行 + Redis 摘要含新事实；
- **down**（ai-svc `LLM_BASE_URL` → `http://127.0.0.1:1/v1`）：消息照发（msg 1046，HTTP 200）+ 落库 rows=1
  + run=`failed`（错误原文 `dial tcp 127.0.0.1:1: connect: connection refused`）+ 该消息 episode=0（待恢复）；
- **up**（恢复 recreate）：新消息（1047）run=`done|1` + episode 补齐；1046 留 failed（重试语义如实记录）。

**IAB 跨会话记忆（生产构建 web + 真人链路）**：
1. 「我的记忆」页关系图出现新节点「最近开始学萨克斯风，每周三晚上上…」（canvas 渲染；
   截图 `iab-cl08c-memory-page-2026-10-24.png`）；
2. **全新会话 774**（与埋事实的会话 771/772/773 无任何同会话历史）问「还记得我最近在学什么乐器吗？」
   ⇒ AI 回复：**「你在学萨克斯风，每周三晚上上课，还会给老师交练习录音」**——三个事实来自三个不同会话
   （base 的乐器+上课时间、up 的练习录音），连用户称呼都正确
   （截图 `iab-cl08c-cross-session-recall-2026-10-24.png`）。

## §5 范围外发现（只记录）

- IAB 会话页出现「3D模型加载失败」（数字人 VRM）——已知账 `CL-F-17` 族，非本轮范围。
- IAB 实测中 locator click 按钮多次超时（登录/发送），需 CUA 坐标 + `cua.type` 真键入才驱动 Vue 状态
  （fill 不触发 v-model）——测试方法学，已进会话记忆。

## §6 未做 / 下一步

- **Phase 4 = 图 + `proto/memory.proto` + 补 CL-F-56**（动 proto 前置；llm-service 侧 graphiti/falkordblite）。
- T-6（实体消解）/T-13/T-14（图后端/三层召回）依赖 Phase 4。
- dev 栈**仍在运行**（:cl08c × 3 + 公共 tag），栈锁释放见登记行交代。
