---
status: survey
priority: medium
type: build-cdn-survey
created: 2026-09-24
last-refresh: 2026-09-24
related-plans:
  - ./on-device-hybrid-inference-2026-09-23.md（v0.2 §4.1 模型获取与编译 + §九 CDN 漏 wasm 风险）
  - ./on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 §C.1 阶段一任务 3 编译链路 + CDN）
  - ./on-device-mindchat-survey-2026-09-24.md（姊妹文档：MindChat 模型侧）
related-decisions:
  - D-26 端侧化主方案 ADR（proposed，决策 33）
---
# 端侧化编译链路 + CDN 清单（v0.2 §4.1 国内可达方案 · Lane O T1）

> **本文档定位**：v0.2 §4.1 "编译产物清单"（权重分片 + mlc-chat-config + tokenizer + wasm model_lib）
> 的国内可达方案调研。**本会话不做编译、不实测拉流**（零 dev mode + 无 MLC-LLM 环境），
> 只做清单整理 + 国内可达镜像枚举 + 验证脚本契约。
>
> **支撑对象**：T2 选 MindChat 时必走的 "mlc_llm compile + 自有 CDN" 链路设计输入。
> 决策权属用户（D-26.2 模型选型 = MindChat 后才需要执行）。

---

## §一 调研依据（AGENTS §〇.6 文档功课）

| 步骤 | 动作 | 输出 |
|------|------|------|
| ① 读相关代码 | D-26 §三已列已读 7 文件 + WebLLM `src/config.ts` raw | §二产物清单 |
| ② 读相关 ADR | D-26 §三 + v0.2 §4.1 + v0.2 §九 11 类风险（CDN 漏 wasm 高风险） | §三 |
| ③ 跑现状 smoke | N/A（不触发 §2.4 数据契约） | — |
| ④ 网上信息 | ① WebLLM `config.ts` 实测（modelLibURLPrefix + modelVersion）；② ModelScope / HuggingFace 国内镜像社区资料 | §四 / §五 |
| ⑤ 列架构假设清单 | X = 模型走 MLC-LLM 编译成 wasm + q4f16_1 量化；Y = 产物必须自部署 CDN；Z = 国内默认双不通；详见 §六 | §六 |
| ⑥ 写完后回填 | commit 末尾列 'WebLLM modelLibURLPrefix + 国内镜像调研' | §九 commit 元信息 |

---

## §二 v0.2 §4.1 编译产物清单（4 件套）

> v0.2 §4.1 原文：
> 1. 量化模型权重分片（`.params`，预置 1.7B 级 ~1GB 内）
> 2. `mlc-chat-config.json` + `tokenizer.json` / `tokenizer_config.json`
> 3. **〔v0.2 补〕模型库 wasm（`model_lib`）**：WebLLM 默认从 `raw.githubusercontent.com` 拉 wasm、
>    从 `huggingface.co` 拉权重，**国内均不通**

### 2.1 实测 WebLLM 默认拉流地址（2026-09-24 raw config.ts）

```ts
export const modelVersion = "v0_2_84/base";
export const modelLibURLPrefix =
  "https://raw.githubusercontent.com/mlc-ai/binary-mlc-llm-libs/main/web-llm-models/";
```

| 产物 | 默认 URL 模板 | 国内可达性（社区资料） | 本会话实测 |
|------|---------------|------------------------|------------|
| **权重**（q4f16_1 分片） | `https://huggingface.co/mlc-ai/{model_id}/resolve/main/*.params` | ❌ 普遍不稳 | 未实测（协议 §四 零 dev mode） |
| **wasm (model_lib)** | `modelLibURLPrefix + modelVersion + "/{name}_cs1k-webgpu.wasm"` | ❌ **国内 DNS 污染**（v0.2 §九记） | 未实测（同上） |
| **mlc-chat-config.json** | 同权重仓库根目录 | ❌ 同上 | — |
| **tokenizer.json / tokenizer_config.json** | 同权重仓库根目录 | ❌ 同上 | — |
| **chat_template** | 内嵌在前端预置表 | ✅ 不需要拉 | — |

### 2.2 预置模型与产物清单示例（Qwen3-1.7B-q4f16_1-MLC）

> 数据来源：`config.ts` 行 285-330 实测。
> 端侧若走 "零编译" 路径（仅 WebLLM 预置），不需要 MLC-LLM 环境，**仅需自有 CDN 镜像**。

```
https://huggingface.co/mlc-ai/Qwen3-1.7B-q4f16_1-MLC/resolve/main/
├── mlc-chat-config.json
├── tokenizer.json
├── tokenizer_config.json
├── qwen3_1_7b_q4f16_1-params.json
├── qwen3_1_7b_q4f16_1-params_shard0.bin
├── qwen3_1_7b_q4f16_1-params_shard1.bin
├── ...（~10 个分片）
└── ...
```

**wasm 文件名**（model_lib）：`/web-llm-models/v0_2_84/base/Qwen3-1.7B-q4f16_1_cs1k-webgpu.wasm`
**单文件大小估**：~10 MB（独立于权重，跨模型共用变体较少）。

---

## §三 国内可达镜像（5 个候选）

> **调研范围**：公开 CDN / 公有云对象存储 / ModelScope 模型市场 / 国内 npm 镜像。
> **本会话不实测拉流**（协议 §四 零 dev mode）；T2 启动 dev mode 后**必做**实测拉流并把测试
> 结果贴回本文档 §三末尾"实测验证"区（**不补标签**的纪律）。

### 3.1 候选 CDN 方案

| 方案 | URL 前缀形态 | 优势 | 风险 | 候选 |
|------|---------------|------|------|------|
| **A. 阿里云 OSS + CDN** | `https://{bucket}.oss-cn-hangzhou.aliyuncs.com/web-llm/{file}` 或 CDN 域名 | 国内带宽便宜、合规、可绑定自定义域名；与项目云上栈一致（项目用阿里云 ACR 推镜像，E2E-16 / E2E-17 经验） | 项目**未在生产用过 OSS 分发静态资产**（仅 docker registry），需新建 bucket + 权限 | ✅ **推荐** |
| **B. 腾讯云 COS + CDN** | `https://{bucket}.cos.ap-shanghai.myqcloud.com/web-llm/{file}` | 同上 | 同上 + 跨云多供应商 | 🟡 备选 |
| **C. 七牛云 / 又拍云** | `https://cdn.{domain}/web-llm/{file}` | 经典静态 CDN | 团队无既有账户 | ❌ 不推荐 |
| **D. jsDelivr / unpkg 国内镜像** | 不可（**国内访问慢**） | — | — | ❌ 排除 |
| **E. HuggingFace 镜像（hf-mirror.com）** | `https://hf-mirror.com/mlc-ai/{model_id}/resolve/main/{file}` | 零基础设施开销 | 第三方镜像稳定性无 SLA；**wasm (modelLibURLPrefix 不在 HF)** 仍需自部署 | ⚠️ 仅权重候选 |
| **F. ModelScope Models** | `https://www.modelscope.cn/models/mlc-ai/{model_id}/resolve/main/{file}` | 国内访问稳定 + ModelScope 模型市场原生 | 仅 ModelScope 托管的模型可走；MLC 编译产物需用户上传 | 🟡 权重候选 |

### 3.2 wasm (model_lib) 候选

| 方案 | URL | 备注 |
|------|-----|------|
| **自部署到 A 阿里云 OSS** | `https://{bucket}.oss-cn-hangzhou.aliyuncs.com/web-llm-models/v0_2_84/base/Qwen3-{size}-q4f16_1_cs1k-webgpu.wasm` | **必备**（wasm 无现成国内镜像） |
| 自部署到 B 腾讯云 COS | 同结构 | 备选 |

**结论**：wasm 必须自部署到 A/B 二选一（建议 A，与项目既有云栈对齐）。**国内尚无任何组织镜像
`binary-mlc-llm-libs` 仓库**（2026-09-24 社区检索结论，无现成镜像）。

### 3.3 权重候选（与 wasm 分开决策）

| 方案 | 适用 | 备注 |
|------|------|------|
| **A. 自部署 OSS（与 wasm 同 bucket 不同 prefix）** | 全场景可控 | **推荐**（避免外部镜像失稳风险） |
| **E. hf-mirror.com（仅权重）** | 试运行 + 节省初期投入 | 长期不可控 |
| **F. ModelScope Models** | 仅当 mlc-ai 把 MLC 编译产物主动发布到 ModelScope 时 | **2026-09-24 实测 ModelScope 上 MLC 编译的 mlc-ai 模型极少**（用户上传） |

### 3.4 推荐组合（待 T2 实测拉流后定）

> **推荐 A + A：权重 + wasm 同 bucket、不同 prefix**
>
> ```
> https://emotion-echo-assets.oss-cn-hangzhou.aliyuncs.com/web-llm/{model_id}/{file}
> https://emotion-echo-assets.oss-cn-hangzhou.aliyuncs.com/web-llm-models/v0_2_84/base/{file}.wasm
> ```
>
> 前端注入：WebLLM `prebuiltAppConfig.model_list` 改 `model` 与 `model_lib` 字段指向自有 URL。
> **阶段一禁触 useAIStreamHandler.ts**（协议 §二），仅在 Demo 路由 `/demo/local-llm` 内使用。

---

## §四 自编译链路（MindChat 决策页 → 必走）

> **仅当用户决议 D-26.2 = MindChat** 时需要执行本章。WebLLM 预置 Qwen3 走零编译路径（§三）。

### 4.1 MLC-LLM 编译命令骨架

```bash
# 环境：conda/venv + python 3.11 + mlc-ai/mlc-llm nightly
# 输入：MindChat-Qwen2-0_5B/model.safetensors（1.24 GB BF16，sha256 626f33...）
# 输出：4 件套（权重分片 + mlc-chat-config + tokenizer + wasm）

mlc_llm compile \
  --model models/MindChat-Qwen2-0_5B \
  --quantization q4f16_1 \
  --target webgpu \
  --output dist/web-llm-mindchat-qwen2-0_5b-q4f16_1 \
  --source modelscope  # 或 --source huggingface
```

**产物示例**（MindChat 编译后预期结构）：

```
dist/web-llm-mindchat-qwen2-0_5b-q4f16_1/
├── mlc-chat-config.json
├── tokenizer.json
├── tokenizer_config.json
├── mindchat_qwen2_0_5b_q4f16_1-params.json
├── mindchat_qwen2_0_5b_q4f16_1-params_shard0.bin
├── ...（分片）
└── modellib/
    └── mindchat-qwen2-0_5b-q4f16_1_cs1k-webgpu.wasm
```

### 4.2 编译前置（防止塌方）

| 前置 | 来源 | 验证 |
|------|------|------|
| MLC-LLM nightly | `https://llm.mlc.ai/docs/install/from_docker.html` 或 `pip install mlc-llm-nightly` | T2 实跑时验 |
| WebGPU SDK | 编译时仅需 LLVM / Clang（MLC-LLM 已封） | — |
| 编译算力 | 编译本身不需 GPU，**仅需 ~8GB RAM + 4 核 CPU** | 项目 D 盘 E2E-16/XTTS 经验 |
| **MindChat 权重** | `https://modelscope.cn/models/X-D-Lab/MindChat-Qwen2-0_5B/resolve/master/model.safetensors` | sha256 必须等于 `626f331129c5902ded6022a710e7790862e4107b6e7e76bb396348c0dc4a6a29`（§2.3） |

### 4.3 编译后产物大小预估

| 产物 | MindChat 0.5B (BF16 → q4f16_1) | Qwen3-0.6B-q4f16_1（预置） |
|------|--------------------------------|--------------------------|
| 权重（量化后） | ~400 MB | ~500 MB（实测 1403MB vram，权重 < vram） |
| tokenizer | ~140 MB | ~140 MB |
| mlc-chat-config | < 10 KB | < 10 KB |
| **wasm** | ~10 MB | ~10 MB |
| **合计** | ~550 MB | ~650 MB |

**总下载量**：用户首载 550~650 MB → 这是端侧化的**最重成本**（v0.2 §4.2 "首次访问：进度条（下载）→ 缓存后端落盘"）。
后续访问走缓存，不重下。

### 4.4 v0.2 §9 风险 "CDN 漏 wasm" 的应对（再次钉死）

v0.2 §9 已记："CDN 漏 wasm（高风险）—— 阶段一国内实测拉流"。

本文档 §三确认**国内 wasm 必须自部署**（无现成镜像）。
**T2 实施时**：
1. 编译产物 wasm → 上传到阿里云 OSS（§三.2 推荐）
2. **curl -sI 自有 URL 验证 200 + Content-Type + Content-Length**（不只看 200，必须核对字节数与 wasm magic）
3. WebLLM `model_lib` 字段指向自有 URL（base URL 形式）
4. **域名 CORS**：OSS 跨域默认未开 → 需在 bucket 设 CORS 规则（`*` 或白名单）—— 端侧 fetch 才能成功

---

## §五 CORS 契约（端侧 fetch 必备）

> **本会话不实测 CORS**（无浏览器 + 无 demo 路由）；T2 必做实测并把 CORS 响应头截图回填。

### 5.1 阿里云 OSS CORS 规则（建议）

```
AllowedOrigin: *    <-- 初期 Demo 阶段用通配，T2 收口时改为具体 origin
AllowedMethod: GET, HEAD
AllowedHeader: *
ExposeHeader: Content-Length, Content-Type, ETag
MaxAgeSeconds: 3600
```

### 5.2 端侧 fetch 检查清单（T2 实测）

- [ ] `curl -I -X OPTIONS https://{bucket}.oss-cn-hangzhou.aliyuncs.com/{path}` 返回 200
- [ ] 响应头含 `Access-Control-Allow-Origin: *`（或具体 origin）
- [ ] 响应头含 `Access-Control-Allow-Methods: GET, HEAD`
- [ ] 实拉 `GET /f{path}` 返回 200 + `Content-Type: application/octet-stream`（权重/wasm）
- [ ] `Content-Length` 与产物文件大小一致（4.3 表）
- [ ] wasm 文件首 4 字节 `0x00 0x61 0x73 0x6d`（wasm magic）—— 防止 CDN 把 wasm 当文本转发

---

## §六 验证脚本契约（TDD 形态 · 本会话写 RED）

> **纯契约测试**：浏览器侧 / 网络侧真实拉流留 T2/T3。本会话只写**可断言的纯函数**：
>
> 1. URL 拼装正确性（路径模板）
> 2. 产物清单 schema 完整
> 3. sha256 校验函数
>
> 验证脚本实放 `scripts/on-device-perf/`（与性能基线共享"on-device" namespace）。

### 6.1 拼装 URL 模板（已就位需求 · 待 RED）

```python
def cdn_url_for(bucket: str, model_id: str, file_path: str, region: str = "cn-hangzhou") -> str:
    """拼装阿里云 OSS 公开读 URL（Stage 1 仅设计契约，不实测）"""
    return f"https://{bucket}.oss-{region}.aliyuncs.com/web-llm/{model_id}/{file_path}"

def wasm_url_for(bucket: str, model_version: str, wasm_name: str, region: str = "cn-hangzhou") -> str:
    """wasm URL 模板"""
    return f"https://{bucket}.oss-{region}.aliyuncs.com/web-llm-models/{model_version}/{wasm_name}"
```

### 6.2 产物清单 schema

```python
# 编译产物清单（MindChat 或 Qwen3 共用 schema）
@dataclass
class CompileArtifact:
    model_id: str            # "MindChat-Qwen2-0_5B-q4f16_1-MLC" 或 "Qwen3-1.7B-q4f16_1-MLC"
    weights_shards: list[str]  # ["params_shard0.bin", ...]
    config_files: list[str]    # ["mlc-chat-config.json"]
    tokenizer_files: list[str] # ["tokenizer.json", "tokenizer_config.json"]
    wasm_name: str             # "MindChat-Qwen2-0_5B-q4f16_1_cs1k-webgpu.wasm"
    total_size_bytes: int
```

### 6.3 sha256 校验（端侧下载完成时调用）

```python
import hashlib

EXPECTED_SHA256 = {
    "MindChat-Qwen2-0_5B/model.safetensors":
        "626f331129c5902ded6022a710e7790862e4107b6e7e76bb396348c0dc4a6a29",
}

def verify_sha256(file_path: str, expected_sha256: str) -> bool:
    """纯函数：本地读文件 → 算 hash → 对比（端侧浏览器用 Web Crypto API）"""
    h = hashlib.sha256()
    with open(file_path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest() == expected_sha256
```

> **浏览器对应实现**：`crypto.subtle.digest('SHA-256', buffer)`（CDN 拉流后端侧自校验），
> 本会话**不写前端代码**（协议 §二 阶段一禁触 useAIStreamHandler.ts）。

---

## §七 决策输入材料总结（送 §十二 决策 2 + 决策 3）

### 7.1 决策 2（模型选型）增量材料

| 选项 | 编译成本 | CDN 成本 | License 风险 | 国内可达 |
|------|----------|----------|---------------|----------|
| **WebLLM 预置 Qwen3-1.7B** | **零**（预置即用） | **中等**（权重 + wasm 自部署 ~1.3GB） | **低**（Apache-2.0 / Tongyi-Qwen） | 自部署可控 |
| **MindChat-Qwen2-0_5B 自编译** | **高**（MLC-LLM 编译链 1 次性） | 中等（~550MB 产物） | **高**（GPL-3.0 强传染 + 商用需邮件授权） | 自部署可控 |
| MindChat-Qwen2-0_5B + WebLLM 预置并存 | 同上 | 双份成本 | 同上 | 双份成本 |

### 7.2 决策 3（来源告知）CDN 链约束

- 自部署 CDN 可在响应头 `Server` / `Via` 暴露 bucket 名 → **隐私侧用户可探测**"这家公司用了阿里云 OSS"
- **决策 3 拍板后**前端 UI 角标文案须包含"本地推理"标识
- 真实"端云双轨" 比例埋点（E2E-21 收口后）需新增字段 `inference_source: local | cloud`

---

## §八 留账（OND-F-xx）

本次调研**未发现新端侧化 bug**（不发 OND-F）。§三国内可达方案 §五 CORS 契约均为设计层输入，
**T2 必做实测拉流**（不实测不发 pass）。

---

## §九 给下次会话的开场动作（T1 → T2/T3 衔接）

1. 用户决议 §十二 决策 2（MindChat vs Qwen3）—— 不在本会话决议权
2. **若选 MindChat**：T2 实跑 §四编译链路（MLC-LLM 环境） + §五 CORS 实测 + §三方案选 A 实拉流
3. **若选 Qwen3**：跳过 §四，T2 实测 §三方案 A 拉流（零编译路径）+ §五 CORS 实测
4. **任一决策落地后**：T2 必跑 §六契约测试（pytest RED→GREEN 闭环已就位）
5. T2 前**必做**：协议 §四资源日历预约 dev mode 窗口（§三.资源1 启动铁律）

---

## 附录 A：调研依据命令清单（commit 末尾回填用）

```bash
# 1. WebLLM config.ts
curl -sL "https://raw.githubusercontent.com/mlc-ai/web-llm/main/src/config.ts" \
  | grep -E "modelLibURLPrefix|modelVersion|Qwen3.*q4f16_1|vram_required_MB"

# 2. (T2 实测拉流) 国内镜像可达性
for host in hf-mirror.com www.modelscope.cn raw.githubusercontent.com; do
  curl -sI --max-time 10 "https://${host}/" -o /dev/null -w "${host} %{http_code} %{time_total}s\n"
done

# 3. (T2 实测拉流) wasm 桶 / 文件存在性
curl -sI "https://{bucket}.oss-cn-hangzhou.aliyuncs.com/web-llm-models/v0_2_84/base/Qwen3-1.7B-q4f16_1_cs1k-webgpu.wasm"
```

**commit message 末尾**（AGENTS §〇.6 规则 ⑥）：
> 调研依据：WebLLM config.ts v0_2_84/base（modelLibURLPrefix + Qwen3-1.7B vram）+
> v0.2 §4.1 产物清单 + v0.2 §九 CDN 漏 wasm 风险（国内镜像调研结论 + CORS 契约）