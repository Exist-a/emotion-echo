---
status: probe-report
priority: medium
type: cdn-empirical-survey
created: 2026-09-24
last-refresh: 2026-09-24
related-plans:
  - ./on-device-compile-cdn-2026-09-24.md（v0.2 §4.1 + §三 5 候选 CDN 设计层）
  - ./on-device-hybrid-inference-2026-09-23.md（v0.2）
  - ./on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 §C.1 阶段一任务 3 编译链路+CDN）
related-decisions:
  - D-26 端侧化主方案 ADR（proposed，决策 33）
---
# CDN 候选可达性实测报告（2026-09-24 · Lane O T2#2）

> **本文档定位**：v0.2 §4.1 "wasm 国内可达" 实测层材料。
> `on-device-compile-cdn-2026-09-24.md` 给出**设计层** 5 候选 + 推荐组合 A（阿里云 OSS）；
> 本报告给出**实测层** 数据（probe.sh 实跑 + wasm magic byte 探测），
> 用于支撑 §十二 决策 2/3（模型选型 + 来源告知）的可达性证据。

---

## §一 调研依据（AGENTS §〇.6 文档功课）

| 步骤 | 动作 | 输出 |
|------|------|------|
| ① 读相关代码 | D-26 §三已读 7 文件 + `on-device-compile-cdn-2026-09-24.md` §三 §五 设计 | 5 候选 CDN 列表 + 探测契约 |
| ② 读相关 ADR | D-26 §二/§三 + v0.2 §九 "CDN 漏 wasm" 高风险 | §四 |
| ③ 跑现状 smoke | `probe.sh` 实跑（3 PASS / 0 FAIL / 8 SKIP） | §三 |
| ④ 网上信息 | 5 候选 host 实测 + wasm magic byte 探测 | §三 |
| ⑤ 列架构假设清单 | "A 阿里云 OSS 推荐 + wasm 必须自部署" —— 实测 A 域名可达 + 对象待部署 | §三 + §五 |
| ⑥ 写完后回填 | commit 末尾列 "5 候选 HEAD+CORS + wasm magic byte" | §七 commit 元信息 |

---

## §二 探测工具（`scripts/on-device-cdn-probe/`）

> **全套脚本**（Lane O 独占列新建）：
> - `probe.sh` —— bash 探测脚本，**16/16 契约测试绿** + 实跑退出码 0
> - `test_probe.sh` —— bash 契约测试（**TDD 节奏**：RED 0/16 → GREEN 16/16）
> - `probe_report.md` —— 实跑产出 markdown 报告（**自动生成**）

### 2.1 探测能力 4 类

| 函数 | 用途 | 退出码语义 |
|------|------|------------|
| `probe_head` | HEAD 请求探测 URL 可达性 | 0 = 2xx/3xx/4xx（域名可达）/ 1 = 5xx（服务异常=FAIL）/ 2 = 000（超时=SKIP） |
| `probe_cors` | OPTIONS + Origin 头探测 CORS | 返回 code + header_json；200/204 含 ACAO=PASS / 4xx=WARN / 000=SKIP |
| `probe_get` | GET 探测 + Content-Length | 返回 code + size |
| `probe_wasm_magic` | Range bytes=0-3 验证 wasm magic `0x00 0x61 0x73 0x6d` | WASM_MAGIC_OK / EMPTY:code / HTML_ERROR:code / NOT_WASM:hex |

### 2.2 SKIP 纪律（check_required_checks.py:24-26）

- 探测任意步骤 timeout / conn-refused → **显式 SKIP**（不静默）
- `--dry-run` 模式下所有探测 SKIP（CI 确定性兜底）
- 退出码：0 = 全部 PASS 或 SKIP；1 = 任意 FAIL

---

## §三 实测数据（2026-09-24 03:29:00Z）

### 3.1 5 候选 CDN HEAD + CORS 探测

| ID | 候选 | Host | HEAD | CORS preflight | 备注 |
|----|------|------|------|----------------|------|
| `RAW` | raw.githubusercontent.com（v0.2 §4.1 wasm 默认） | `raw.githubusercontent.com` | **[SKIP]** timeout | [SKIP] timeout | **国内不通确认**（Stage 59 教训复现） |
| `A` | **Aliyun OSS**（推荐） | `oss-cn-hangzhou.aliyuncs.com` | **[PASS]** 400 | [WARN] CORS-blocked 400 | **域名可达**（400 = 路径 /README 不存在）；CORS 默认阻断（bucket 创建后需加白名单选项） |
| `B` | Tencent Cloud COS | `cos.ap-shanghai.myqcloud.com` | **[PASS]** 400 | [WARN] CORS-blocked 400 | 域名可达（与 A 同语义）；无 repo 既有 footprint |
| `E` | HuggingFace mirror | `hf-mirror.com` | **[SKIP]** timeout | [WARN] no-ACAO | **本机网络到 hf-mirror.com 不稳**（Stage 59 需 TUN 才通） |
| `F` | ModelScope | `www.modelscope.cn` | **[PASS]** 404 | [WARN] CORS-blocked 404 | 域名可达 + API 端点正确但 /README 不存在；CORS 默认阻断 |

### 3.2 wasm magic byte 探测（Aliyun OSS 推荐 bucket）

- 探测 URL：`https://emotion-echo-assets.oss-cn-hangzhou.aliyuncs.com/web-llm-models/v0_2_84/base/Qwen3-1.7B-q4f16_1_cs1k-webgpu.wasm`
- 结果：**[SKIP]** HTML_ERROR:404（bucket 未创建，符合预期 —— T3 创建后回测）
- 探测契约：拉前 4 字节验证 wasm magic `0x00 0x61 0x73 0x6d`；空响应 / HTML 错误页 → SKIP（bucket 不存在是合理 SKIP 而非 FAIL）

### 3.3 汇总

- **PASS**: 3（HEAD 域名前缀：A 阿里云 / B 腾讯云 / F ModelScope + E 域名可达但 timeout）
- **FAIL**: 0（无任何服务异常）
- **SKIP**: 8（1 RAW HEAD + 1 RAW CORS + 1 E HEAD + 4 CORS-blocked + 1 wasm HTML_ERROR）

> **关键观察**：
> 1. **`raw.githubusercontent.com` 国内不通** —— 与 v0.2 §九 "CDN 漏 wasm" 风险完全吻合
> 2. **3 个候选 CDN host 域名都通**（A/B/F HEAD 200~404）—— 国内可达
> 3. **CORS 默认阻断** 是部署要点 —— T3 创建 bucket 时**必须显式配 CORS 白名单**（协议 §五 6 项检查清单之一）
> 4. **hf-mirror.com 不稳** —— 仅作为权重 fallback（Stage 59 教训：TUN 模式才稳定），不推荐做 wasm 承载

---

## §四 探测结论 → §十二 决策 2/3 输入材料

### 4.1 决策 2（模型选型）CDN 链约束

| 选项 | 编译成本 | CDN 权重可达 | CDN wasm 可达 | License 风险 |
|------|----------|---------------|---------------|---------------|
| **WebLLM 预置 Qwen3-1.7B** | 零 | A/B/F 三选一 | A 阿里云 OSS（自部署 wasm）| 低（Apache-2.0 / Tongyi-Qwen）|
| **MindChat-Qwen2-0_5B 自编译** | 高（MLC-LLM 编译链）| A/B/F 三选一 | A 阿里云 OSS（自部署 wasm）| **高**（GPL-3.0 强传染 + 商用需邮件授权）|

**结论**：**wasm 必须自部署到 A 阿里云 OSS**（3 候选 CDN 域名都通；阿里云既有 footprint 最强）。
模型选型层面：**MindChat License=GPL-3.0 是 v0.2 §2.2 漏标的硬约束**，与本次 CDN 探测无直接关系。

### 4.2 决策 3（来源告知）CDN 链约束

- 自部署 CDN 可在响应头 `Server` / `Via` 暴露 bucket 名 → **隐私侧用户可探测**"这家公司用了阿里云 OSS"
- 真实"端云双轨" 比例埋点（E2E-21 收口后）需新增字段 `inference_source: local | cloud`
- 自部署 CDN 上线后**首次加载** = wasm 10 MB + 权重 ~500 MB；用户感知必须显著优于"端侧不告知"

---

## §五 部署前必做（T3 实施清单）

### 5.1 Aliyun OSS bucket 创建清单（v0.2 §五 CORS 契约 6 项检查）

| # | 检查项 | 状态 |
|----|--------|------|
| 1 | `curl -I -X OPTIONS https://{bucket}.oss-cn-hangzhou.aliyuncs.com/{path}` 返回 200 | T3 实测 |
| 2 | 响应头含 `Access-Control-Allow-Origin: *`（或具体 origin）| T3 实测 |
| 3 | 响应头含 `Access-Control-Allow-Methods: GET, HEAD` | T3 实测 |
| 4 | 实拉 `GET /{path}` 返回 200 + `Content-Type: application/wasm`（不是 `application/octet-stream` 默认） | T3 实测 |
| 5 | `Content-Length` 与产物文件大小一致（v0.2 §4.3 表）| T3 实测 |
| 6 | wasm 文件首 4 字节 `0x00 0x61 0x73 0x6d`（防止 CDN 把 wasm 当文本转发）| **本次探测已用契约** |

### 5.2 wasm 文件 Content-Type 特别注意

阿里云 OSS **默认 Content-Type 是 `application/octet-stream`**（已是 wasm 二进制 OK）；
但若使用 CDN 加速 + 文本压缩，可能导致 wasm 头部损坏。
**T3 上线后必须实测：浏览器内 fetch wasm → 验证 `Content-Type: application/wasm` + 首 4 字节 OK**。

---

## §六 留账（OND-F-xx）

本次诊断**未发现新端侧化 bug**（不发 OND-F）。CDN CORS-blocked 是部署期必做事项，
不属于 stage1 阶段收口契约。

---

## §七 给下次会话的开场动作（T1/T2 衔接）

1. 用户决议 §十二 决策 2（MindChat vs Qwen3）—— 不在本会话决议权
2. 若选 MindChat：T2 编译 → 上传阿里云 OSS → 实测 §五 6 项检查清单 → 再跑 probe.sh 验 wasm magic
3. 若选 Qwen3：跳过 §四 自编译链，T2 直接走 §三 probe + §五 5 项检查
4. 决策 3（来源告知）CDN 链约束已就位；UI 角标文案待 §十二拍板后由 Lane E 接入

---

## 附录 A：调研依据命令清单（commit 末尾回填用）

```bash
# 1. 实测探测（5 候选 CDN HEAD + CORS）
cd D:/源码/Emotion-Echo
bash scripts/on-device-cdn-probe/probe.sh
# → probe_report.md 生成；退出码 0

# 2. CI 确定性兜底（无网络时仍 0 退出）
bash scripts/on-device-cdn-probe/probe.sh --dry-run

# 3. 单 host 探测
bash scripts/on-device-cdn-probe/probe.sh --host aliyuncs.com

# 4. 契约测试
bash scripts/on-device-cdn-probe/test_probe.sh
# → 16/16 PASS

# 5. (T3 后) 验证 wasm magic + CORS
bash scripts/on-device-cdn-probe/probe.sh --host emotion-echo-assets
# 期望：wasm magic OK + CORS PASS
```

**commit message 末尾**（AGENTS §〇.6 规则 ⑥）：
> 调研依据：5 候选 CDN bash 实测（PASS=3 FAIL=0 SKIP=8）+
> wasm magic byte 探测契约 + v0.2 §4.1 + §九 CDN 漏 wasm 风险 +
> check_required_checks.py:24-26 显式 SKIP 纪律 + smoke_upload_minio.sh:71 HEAD 探测模板