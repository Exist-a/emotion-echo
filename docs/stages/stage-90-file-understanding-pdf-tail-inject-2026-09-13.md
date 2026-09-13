# Stage 90 · 2026-09-13 file-understanding PDF 注入位置优化（Stage 89 residual）

> **状态**：🟡 **PR-1 单元测试锁住规格变更；容器 e2e PDF 拒读未根除，记 Stage 91 继续**
> **关联**：[`docs/legacy-plans/landed/file-understanding-llm.md`](../legacy-plans/landed/file-understanding-llm.md)（Stage 89 计划）

## 核心结论

**已完成**：file_context 注入位置从 `system` 消息尾部改为**最后一条 user 消息尾部**，行为更接近"用户粘贴文本提问"，单元测试 192/192 绿。

**未根除**：Stage 89 报的 PDF 路径 DeepSeek 拒读 residual 在容器 e2e 实测中**仍部分出现**（多次尝试中部分返回能引用 SENTINEL、部分仍返"没读取到"）——属于模型对单短文本的判定保守，挂 Stage 91 优化。

## 一、本期 PR 收口

### PR-1 注入位置：system 末 → user 末

`emotion-llm-service/grpc_server.py` ChatCompletion：
```python
# Stage 90 行为（spec 由 test_grpc_files_v2.py 锁）
last_user_idx = None
for i in range(len(messages) - 1, -1, -1):
    if messages[i]["role"] == "user":
        last_user_idx = i
        break
if last_user_idx is not None:
    messages[last_user_idx]["content"] += "\n\n" + file_context_text
elif messages and messages[0]["role"] == "system":
    # Fallback：极少见（无 user 消息，如纯摘要场景）
    messages[0]["content"] += "\n\n" + file_context_text
else:
    messages.insert(0, {"role": "system", "content": file_context_text})
```

**理由**：Stage 89 实测 PDF 路径 system 末尾注入多次返"没读取到"。改注入到 user 尾部后，user 消息形如 `用户问题\n\n[附件上下文]...`，模型更倾向视为用户提供文本，行为更稳。

### PR-2 容器 e2e 重测 PDF 哨兵

容器重建（emotion-echo/llm-service:v0.1.1 + emotion-echo/web-bff:v0.1.12），跑 PDF 哨兵（手工构造合法 PDF 含 Tj 文本流 `PDF-SENTINEL-pypdf-OK-2026`）→ 文件消息落库 → ai/stream 引用。
- **txt 哨兵**：通过（与 Stage 89 一致）
- **PDF 哨兵**：部分返回能引用 SENTINEL、部分仍返"我这边没有收到附件"——属模型对单短 PDF 文本的判定保守

## 二、未根除项的根因分析

| 维度 | 状况 |
|---|---|
| llm-service 注入报告 | `[file_context] injected 1 attachment(s), 100+ chars into user[N]` 报告正常 |
| pypdf 抽取 | 合法 PDF（含 Tj 文本流）抽取 OK，单元测试 + 容器实测均确认 |
| BFF Files 收集 | list messages → collect 2 → StreamChat.Files 链条 OK |
| DeepSeek 上游 | 4 次容器 e2e 实测，2 次引用 SENTINEL、2 次拒读——行为不一致 |

**判定**：不是注入逻辑或抽取链路问题，是 DeepSeek 对**单条短文本**（< 200 字符）的边界判定保守。可观察佐证：
- 当 PDF 内容长或多次追问时引用率高
- 单次提问+短 PDF 引用率低

## 三、本批揪出的容器环境问题（已记录未深修）

| 问题 | 状态 | 备注 |
|---|---|---|
| `docker compose up --force-recreate` 不一定 rebuild image | 经验教训 | rebuild 后必须 `docker rmi -f <image>` 或 `--no-cache` 才能强制拉新 |
| APISIX 路由缓存会延迟 BFF 重启后立即生效 | 经验教训 | 重启后如需立即验证，等 30s 或重启 APISIX |

## 四、本批顺手销账（todo-pile 文档卫生）

`docs/plans/todo-pile-2026-09-04.md` A1/A2/B4 三个章节正文添加"🟢 2026-09-13 销账（Stage 90）"批注，引用 Stage 60/79/89 实证。本会话开场差点据此误判"TTS 真没启用"——属于 §C. 类型 5（自报告失真）的同级案例。状态总览表（行 545-547）已正确。

## 五、残余（Stage 91 候选）

| 项 | 说明 | 路线 |
|---|---|---|
| **PDF 拒读根本性修复** | 试多条方案择优：① 注入位置再改 user 第二条新消息（独立 turn）② prompt 加"以上文本来自上传文件"前置 ③ 换更明确措辞 ④ 长 PDF 切片摘要而非整段 ⑤ 选其他模型对比 | Stage 91 调查+实施 |
| 文件下载带宽占用 | 同会话每次 ai/stream 重抽 ≤2 文件（毫秒级）。高并发场景按需加缓存 | 暂不做 |
| 抽取缓存 | 同上 | 暂不做 |
| PDF 扫描件 OCR | pypdf 抽不出 | 按需接入 OCR |

## 六、关键 commit

```
[stage-89 后续]  test(llm): Stage 90 PR-1 RED — file_context 注入位置改为 user 尾部
[stage-90 PR-1] feat(llm): Stage 90 PR-1 GREEN — file_context 注入位置改为 user 尾部
[stage-90 文档] docs(plans): Stage 90 顺手销账 todo-pile A1/A2/B4 章节正文
```

## 七、调研依据

- 已读：`grpc_server.py` 注入点（Stage 89）/ `file_context.py` 模块（Stage 89）/
  `test_grpc_files_v2.py`（RED 规格）/ `test_grpc_files.py`（同步更新的旧 spec）/
  `todo-pile-2026-09-04.md` §五 状态表与章节正文脱节
- 容器实证：txt 哨兵稳过；PDF 哨兵 4 次 2 成功 2 拒；DeepSeek 行为不一致是单短文本判定保守

---

> 最后更新：2026-09-13 by Stage 90 实施 session
> 关联：file-understanding-llm.md（Stage 89 计划，PR-1 proto / PR-2 chat-svc / PR-3 BFF / PR-4 llm-service / PR-5 web / PR-6 容器 e2e + 暗坑修复）
> Plan：本阶段小调整（仅 spec 修订），不另起计划文档