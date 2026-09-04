# ADR · 文档失真治理（Documentation Drift Registry）

- **编号**：决策 18
- **日期**：2026-09-04
- **状态**：✅ 生效
- **相关**：[decisions.md](/docs/architecture/decisions.md) · [AGENTS.md §0.2](/AGENTS.md) ·
  [stage-38-system-status.md](/docs/stages/stage-38-system-status.md) ·
  [stage-39-nacos-enablement.md](/docs/stages/stage-39-nacos-enablement.md)

---

## 一、背景

`docs/architecture/decisions.md` 开篇声明自己是"单一事实源"，并要求
"所有 stage 文档、路线图、代码组织、配置都应与本文档一致"。
但项目实际长期存在**文档与代码不符**的问题，AGENTS.md §0.2 已为此加过一道
"写文档前必须先读代码 / 查 ADR / 跑 smoke"的前置闸门。

该闸门约束的是**新写**的文档，没有解决两个遗留问题：

1. **存量失真无人清点**。Stage 38 §四 曾把此事记为"ADR 与代码失真累计（至少 3 处），
   待 ADR-20 立项"，但该 ADR 一直没建，"至少 3 处"也从未展开成清单。
2. **失真会被下游文档继承**。一处错误结论被后续 stage 引用后，
   修代码的人按错文档走，产生二次错误（如 A1 修复方向定错、A4 修 GRANT 但视图没建）。

2026-09-04 一次合并前复核，在两天内的文档里又发现 **6 处失真**——
失真产生速度已超过修正速度，需要立决策而非继续零散修补。

## 二、已登记的失真实例

以下 6 条均为 2026-09-04 实测发现并已就地更正。列出来不是为了追责，
而是为了归纳出失真的**类型**（§三），以便设计防线。

| # | 出处 | 文档写的 | 实测事实 | 类型 |
|---|---|---|---|---|
| 1 | stage-39 §七.3 | llm-service 缺 `nacos_client`，需补 `requirements.txt`（`nacos-sdk-python` 之类） | 该依赖 `requirements.txt:8` 早就有，`nacos_client.py` 也在仓库里；真因是 `Dockerfile` 两阶段逐个 COPY 时漏了这个文件 | 根因臆断 |
| 2 | stage-39 §六 | `analytics-svc/internal/trigger` 是"预存在 FAIL" | `go test -count=1` 实测 `ok 1.559s` | 陈旧结论 |
| 3 | stage-39 §六 | `emotion-echo-web-bff: exit 0` | 实际 exit 1，4 条断言红（§4.2 的验证脚手架被 commit 进 `e3c662d`） | 未复跑即记录 |
| 4 | stage-39 §七.5 / stage-38 §三阻断 5 | 4 个 Go svc 报 unhealthy | `docker ps` 六个业务容器全部 `(healthy)` | 陈旧结论 |
| 5 | stage-38 §四隐患 2 | `daily_emotion_by_modality_v` 因表名是 `face_detections`/`voice_transcripts` 而建不出 | 与表名无关；依赖的表在未应用的 ai-svc migrations 002/003 里，按序应用后 `CREATE VIEW` 直接成功 | 根因臆断 |
| 6 | stage-38 §三阻断 2/3 | 文件上传、多模态"路由 404，待查路由是否真在 BFF 挂了" | 路由都挂了。真实路径是 `/api/v1/uploads/:kind`、`/api/v1/multimodal/analyze`；多模态打对路径返 `code:0` 完全正常，上传返 502（有意占位） | 探测方法错误 |

附带被低估的一项（非失真，但严重度记错）：stage-38 §四隐患 1
"migration 未挂 initdb.d（🟡 重建 dev 会丢表）"，实际是**当时的库就已经缺**，
且范围是全部 13 个服务迁移而非 3 个，smoke 一度只剩 1/10 PASS。

## 三、失真的四种类型与共性

归纳上表，失真集中在四类：

1. **根因臆断**（#1 #5）——观察到现象后直接写下一个"看起来合理"的原因，
   没有去验证该原因是否成立。这类最危险，因为它会直接把修复方向带偏。
2. **陈旧结论**（#2 #4）——某次跑出的结果被当作长期属性记下来，
   后续不再复验。flaky 测试和已被修复的问题都会以"已知问题"的形式僵化在文档里。
3. **未复跑即记录**（#3）——收口文档里的"全绿"表格是凭印象填的，
   没有在写文档的那一刻真跑一遍。
4. **探测方法错误**（#6）——用错误的方式去验证（打了不存在的路径、
   发了服务端不接受的编码），把工具错误当成系统缺陷。

共同点：**都可以被"当场跑一次"证伪**，成本很低，但没人跑。

## 四、决策

### 4.1 结论断言必须附可复现证据

凡在 stage / ADR / plan 文档中写下"某某是 X"的**结论性断言**，
必须同时给出产生该结论的**可复现命令与其原始输出**（命令 + 关键输出行）。
不给证据的结论一律降级为"假设"，并显式标注 `（未验证）`。

理由：上表 6 条里有 5 条只要贴一行实际命令输出就不会写错。

### 4.2 "已知问题 / 预存在 FAIL" 必须带验证时间戳

任何以"预存在""已知""长期如此"措辞记录的问题，必须写明**最后一次验证的日期**。
超过一个 stage 未复验的，后续引用方有义务先复验再引用，不得直接继承。

理由：#2 #4 都是陈旧结论被继承。

### 4.3 端点/路径类结论必须先核对注册表

涉及"某端点 404 / 不存在 / 未实现"的结论，必须先从**代码侧的路由注册处**
确认真实路径，再据此探测。禁止仅凭一次 curl 404 断言功能缺失。

理由：#6 让两项正常/已知的功能被记为阻断，并进入了 P0 修复清单。

### 4.4 失真更正就地标注，不静默改写

发现失真时，**保留原文并就地追加更正块**（注明日期、实测依据、结论变化），
不得直接删改原文。同时在本 ADR 的 §二 追加一行登记。

理由：静默改写会让引用了原结论的下游文档失去追溯线索；
保留原文也才能让 §三 的类型归纳持续有效。

### 4.5 本 ADR 是失真登记的单一入口

后续发现的文档失真统一登记到本文件 §二，不再另开 ADR。
Stage 38 §四隐患 5 提到的"ADR-20"即本决策（当时编号为随手估计，
实际 `decisions.md` 决策序号排到 17，故本决策取 **18**）。

> ⚠️ **注意："ADR-20" 这个占位编号在仓库里被用于两件不相干的事**，
> 本决策只承接其中一件：
>
> | 占位处 | 指代 | 是否被本决策承接 |
> |---|---|---|
> | `stage-38-system-status.md` §四隐患 5、`stage-38-A-landing.md` §六.5、`stage-38-A-dev-apisix-path.md` §4 | **文档失真清单** | ✅ 是，即本决策 18 |
> | `adr-2026-09-dev-publisher-user-behavior-events.md` §95/§180、`stage-37-A-landing.md`、`stage-37-B-landing.md` §四.4 | **chat-svc 表依赖清单**（dev-only 跨服务职责积累到 3+ 时起） | ❌ 否，与本决策无关，**仍是未做项** |
>
> 后者请勿因本决策落地而误认为已收口。

## 五、不做什么

- **不引入文档 lint / CI 校验**。当前失真集中在"事实正确性"而非格式，
  自动化校验无法判断"这个根因是不是真的"，投入产出不合算。
- **不追溯修订 Stage 1~37 的历史文档**。历史 stage 文档的定位是"演进记录"，
  按 §4.4 只在被引用且发现失真时就地更正，不做批量清洗。
- **不改变 AGENTS.md §0.2 的既有闸门**。本 ADR 是它的补充（约束结论质量），
  不是替代（约束动笔前的功课）。

## 六、影响

| 对象 | 影响 |
|---|---|
| stage / ADR / plan 作者 | 写结论时多贴一行命令输出；写"已知问题"时多写一个日期 |
| 引用方 | 见到无证据结论时按"假设"对待，不作为修复依据 |
| AGENTS.md | §0.2 已有的六步功课不变，本 ADR 追加"结论需带证据"的产出要求 |
| 已有文档 | stage-38 / stage-39 已按 §4.4 就地标注完毕 |

## 七、调研依据

- **读过的代码**：`emotion-llm-service/{Dockerfile,main.py,requirements.txt}`、
  `emotion-echo-web-bff/main.go`（`registerRoutes`）、
  `emotion-echo-web-bff/internal/handler/{upload,multimodal,tts}_handler.go`、
  `emotion-echo-web-bff/internal/config/config_test.go`、
  `emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql`
- **查过的文档**：`decisions.md`（决策 1~17）、`stage-38-system-status.md`、
  `stage-39-nacos-enablement.md`、`AGENTS.md` §0.2 / §2.2 / §2.4
- **跑过的验证**：`scripts/smoke_data_layer.py`（1/10 → 10/10 PASS）、
  7 模块 `go test -count=1` + `go vet`、`pytest emotion-llm-service/tests/unit`（105 passed）、
  `docker ps`、`docker logs emotion-echo-llm-service`、
  对 `/api/v1/{uploads/:kind, multimodal/analyze, tts/synthesize}` 的逐条 curl 探测
