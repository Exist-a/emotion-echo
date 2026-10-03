---
stage: e2e-27
title: 对象存储 MinIO（头像上传/下载/匿名读权限）
type: verification
status: in-progress
created: 2026-10-03
last-updated: 2026-10-03（**开工**：状态翻 in-progress，RUNBOOK §1 开工前置三项全过——depends-on 空 / 无决策门 / plan 就绪；执行环境基线 = 全栈 20 healthy 遗留运行态）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；执行期 [M] 决策点见 §4
related-findings: [E2E-F-116]
---

# E2E-27 对象存储 MinIO — 详档（任务书）

> **类型**：verification —— MinIO 对象存储自 Sprint 1 PR-4 落地（`avatars` 桶 + anonymous download + StorageClient 抽象），此后 avatar / uploads / voice 三类对象**共存单桶**，健康与上传契约工具已存在，但它**当前不能证明自己是对的**：
> ① **F-116 存量债未修**——avatar / uploads 响应仍下发 `PublicBaseURL` 绝对地址（运行时 DB 实证见 §0.1 F-b），而 [ADR-2026-09-client-object-url-bff-proxy](../../../architecture/adr/adr-2026-09-client-object-url-bff-proxy.md) 已裁定"面向客户端的对象 URL 一律网关相对 + BFF 反代"——voice 已按此落地（PR #59），avatar/uploads 没有 ⇒ 账本 **E2E-F-116 owner = E2E-27**（🟡 待修）；
> ② **匿名读是桶级的**——init 容器 `mc anonymous set download dev/avatars` 作用于**整个桶**：voice/ 音频与头像/附件同权限，且宿主 `:9000` 是 `0.0.0.0` 暴露 ⇒ **绕过网关 JWT 直读对象**在 dev 是事实可达的（F-f/F-g）——"匿名读权限"边界必须定性并裁定处置；
> ③ **生命周期无治理**——头像更新只 Put 不删旧（uid=1 存量 5 个对象实证 F-d），`RemoveObject` 有实现无调用方；单桶混放三类对象、无生命周期策略；
> ④ **守卫空转**——`check_minio_health.sh`（4 契约）与 `smoke_upload_minio.sh`（4 契约）均**未接 CI**（AP-10 同型：写了守卫没接线）。
> **本阶段把"能存"变成"URL 契约对、匿名读边界清、生命周期有结论、守卫真接线"。**

> **依据**：roadmap 排期总表 E2E-27 行（"头像上传/下载/匿名读权限"，边界列"其他文件类型接入"）+ [ADR-2026-09-client-object-url-bff-proxy](../../../architecture/adr/adr-2026-09-client-object-url-bff-proxy.md)（决策 1 + 后果段明写"avatar / uploads 仍绝对地址 → 同型存量债登记 E2E-F-116 归 E2E-27"）+ 账本 E2E-F-116。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：无依赖前置。上一阶段 E2E-26 ✅ done（2026-10-03，20/20 PASS + 第二方核对通过）。
> **名下账本**：**1 条 —— E2E-F-116**（🟡 待修；2026-10-03 计划期对账：同域条目 F-105 / F-85 / F-86 均已 ✅，仅 F-116 挂本阶段）。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的探针**（AGENTS §〇 文档功课），非引用历史结论。执行期若发现与本节不符，**以实测为准并回填本节**（AP-02）。

### 0.0 假设清单（本文假设，与现状对比见 §0.1）

| # | 本文假设 | 依据 / 现状 |
|---|---------|------------|
| A1 | 验证环境 = **dev compose 单机**；prod 对象存储不在本阶段 | `charts/emotion-echo` 全目录 grep minio **零命中**（F-j）——生产对象存储部署未定义 |
| A2 | F-116 **修复方向已由 ADR 裁定**（网关相对路径 + BFF 反代），执行期只决**范围与形态**（M1），不重开方向 | ADR 决策 1/2 明文；voice 先例已落地 |
| A3 | 单桶 `avatars` + 三前缀（`avatars/` `uploads/` `voice/`）结构维持现状 | `mc ls -r` 实测 5/32/17 对象（F-e）；除非 M2 裁定分桶 |
| A4 | E2E-11 已验的 UI 层头像交互仍有效——本阶段从**存储层 / URL 契约**取证，UI 只做发起者视角验收 | [E2E-11 plan](../e2e-11-my-space/plan.md) 测试点 5/6 已覆盖"上传→存储→页面更新 + 2MB 前端拦截" |
| A5 | MinIO 单实例、dev 凭证 minioadmin/minioadmin；prod 密钥治理（K8s Secret）不在本阶段 | `deploy/docker-compose.infra.yml:264-265` + `deploy/minio/README.md` |

### 已读实现文件（≥3，AGENTS §〇 ①）

- `emotion-echo-web-bff/internal/storage/minio.go`（全文 149 行）——StorageClient 五方法接口 / `ObjectKey` = `avatars/<uid>-<sha8>.<ext>` / `GetObjectURL` = `PublicBaseURL/bucket/key` 拼接 / 注释明写"依赖 bucket 设为 anonymous download" / `GetObject` 反代语义（StatObject→GetObject、404 语义）
- `emotion-echo-web-bff/internal/handler/avatar_handler.go`（全文 130 行）——POST `/api/v1/user/avatar`：X-User-Id 401 / storage nil 503 / `MaxBytesReader`+二次校验 2MB（F-85 修复在位）/ PutObject→UpdateMe / `OK()` 包装（F-86 修复在位）/ **只 Put 不删旧**（注释"此处不删 MinIO"仅指写库失败分支）
- `emotion-echo-web-bff/internal/handler/upload_handler.go`（契约头 + 前 60 行）——`/uploads/:kind` 三 kind 白名单 / mime 白名单（file 不限）/ 大小上限 image 5MB·video 50MB·file 20MB / 415·413·400·503 语义
- `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:272-287`——`fileSourceURL` 按 `PublicBaseURL` **前缀匹配**把消息里存的绝对地址重写为 llm-service 内部端点；前缀不匹配原样返回 ⇒ **F-116 修复与此耦合**（F-c）
- `emotion-echo-web-bff/internal/handler/voice_handler.go`（契约段）——反代先例：`GET /api/v1/voice/audio/:filekey`（F-113 修复，所有环境同路径）
- `emotion-echo-web-bff/main.go:413-425` + `internal/config/config.go:99,205-206,355`——装配与 `PublicBaseURL` 默认 `http://localhost:9000`
- `deploy/docker-compose.infra.yml:258-301`——minio 服务（`quay.io/minio/minio:latest`、`0.0.0.0:9000-9001` 宿主映射、healthcheck `/minio/health/live`）+ minio-init（`mc mb` 幂等分支 + `mc anonymous set download`）

### 已读测试文件（AGENTS §〇 ①）

- `emotion-echo-web-bff/internal/handler/avatar_handler_test.go`（前 110 行 + 契约头）——fakeStorage/fakeUserClient 双替身、401/400/503/500 契约、`fakeStorage.GetObject` 故意返错（"avatar handler 不应调用 GetObject"防护）
- `emotion-echo-web-bff/internal/storage/minio_test.go`（grep 片段）——`GetObjectURL` 三形态（dev/prod/尾斜杠）拼接契约

### 已查 ADR / 决策 / stage（AGENTS §〇 ②）

- **ADR-2026-09-client-object-url-bff-proxy（全文 53 行）**——决策 1 客户端对象 URL 一律网关相对；决策 2 `PublicBaseURL` 只作内部用途；决策 3 反代走网关 jwt-auth + 单段 key 防御 + 404 语义；后果段明写 avatar/uploads 待修归 E2E-27；增补段（对象 URL 持久化契约）给 messages.content 相对化的先例
- **e2e-roadmap/decisions.md**：D-37/38/39（E2E-26）已占用，**本阶段决议从 D-40 起**；无 MinIO 既有决议
- **architecture/decisions.md**：已用至决策 38（Lane O 的端侧条目），**若产生架构级决议从决策 39 起**
- **E2E-11 plan**：头像 UI 链路（测试点 5/6）已验——本阶段不重复 UI 交互，只做存储层与 URL 契约
- **账本对账**：F-116（owner=E2E-27，🟡 待修，本阶段唯一名下条目）；F-105 ✅（uploads data 包装，PR #55）；F-85 ✅ / F-86 ✅（E2E-11 轮修复）；F-113 ✅（voice 反代，PR #59）

### smoke / 运行时探针（AGENTS §〇 ③）

- `bash scripts/check_minio_health.sh` → **4/4 PASS，rc=0**（容器 running / liveness 200 / console 200 / avatars 桶存在）
- 宿主匿名读：`curl http://localhost:9000/avatars/avatars/1-2ec01835.png`（**无任何鉴权**）→ `200 image/png 70B`
- `mc anonymous get d/avatars` → `Access permission for d/avatars is download`（桶级匿名读生效）
- `mc ls -r` 对象清单 → `avatars/ 5` + `uploads/ 32` + `voice/ 17`；含 `uploads/2-961a7db5.txt` **25MiB**（超 file 20MB 上限，F-e）与 `.exe`（kind=file 允许）
- `mc admin info` → **Version: 2025-09-07T16:13:09Z**（与镜像 Created 同日，`:latest` 自 2025-09-07 未再拉取）
- DB 回读：`emotion_echo_user.users.avatar_url = http://localhost:9000/avatars/avatars/1-2ec01835.png`（**绝对地址已持久化**）；`emotion_echo_chat.messages` file 消息 `content = http://localhost:9000/avatars/uploads/t19c.txt`（同型，F-b/F-c）
- **业务契约 smoke（§2.4）不适用**：本 PR 纯文档（plan + roadmap），不触碰业务代码路径——按 AGENTS §〇③ 记录豁免理由
- **外部官方文档（功课④）**：min.io docs 两个 URL 404 + minio/mc GitHub 文档路径 404 + Bocha 配额尽——**检索未获官方原文**；匿名语义改以**运行时 `mc anonymous get` + 匿名 GET 200 实测**为准（比文档更强的行为级证据），如实记录于此

### 0.1 计划期实测事实表

| # | 事实 | 证据（`文件:行号` / 探针输出） |
|---|------|--------------------------|
| **F-a** | **匿名读是桶级策略**：init 对整个 `avatars` 桶 `mc anonymous set download` ⇒ `avatars/` `uploads/` `voice/` 三前缀**同权限**；voice 音频（本应走网关 JWT 的反代路径）在直连 `:9000` 时**无鉴权可读** | `deploy/docker-compose.infra.yml:294` + `mc anonymous get` 实测 `download` + 宿主匿名 GET 200 |
| **F-b** | **F-116 运行时实证**：绝对地址不仅在响应里，**已持久化进 DB**（users.avatar_url + messages.content 两处）⇒ 修复必然牵出存量数据形态问题 | psql 回读两处（§smoke 段原文） |
| **F-c** | **fileSourceURL 耦合**：chat 附件 LLM 抓取按 `PublicBaseURL` 前缀匹配重写为容器内端点；若 M1 改了持久化 URL 形态，前缀失配 → 原样返回 → `FILE_FETCH_ALLOWLIST` 拒绝 → 附件降级 | `ai_stream_handler.go:272-287` + F-b 存量数据 |
| **F-d** | **生命周期无治理**：头像更新只 Put 不删旧——uid=1 存 5 个 `avatars/` 对象（全 70B 测试图）；`RemoveObject` 接口在位但 avatar_handler 无调用 | `mc ls -r` 计数 + `avatar_handler.go:98-121`（无 RemoveObject 调用）+ `minio.go:137-141` |
| **F-e** | **单桶三前缀混放**（5/32/17 对象）；`uploads/2-961a7db5.txt` = **25MiB 超 file 20MB 上限**仍存在 ⇒ 上限执行现状存疑（历史上传 or 绕过，执行期定性）；`.exe` 存在属 kind=file 白名单内 | `mc ls -r` 计数 + `upload_handler.go:54-60` 上限表 |
| **F-f** | **端口 0.0.0.0 暴露**：`9000-9001->9000-9001` 绑所有网卡（对照 E2E-26 D-38 对 sw-ui 的 `127.0.0.1` 限定收紧）——MinIO 未纳入 Stage 33 端口收紧范围 | `docker ps` PORTS 列 + `docker-compose.infra.yml:268-271` |
| **F-g** | **版本未 pin**：镜像 `quay.io/minio/minio:latest` + `mc:latest`，无 digest 无版本号；运行时版本 `2025-09-07T16:13:09Z` | `docker-compose.infra.yml:259,287` + `mc admin info` |
| **F-h** | **守卫未接 CI**：`check_minio_health.sh` / `smoke_upload_minio.sh` 在 `.github/workflows/` grep 零命中（AP-10 同型） | grep 输出为空（本轮实测） |
| **F-i** | **健康与启动链在位**：healthcheck `curl /minio/health/live`；dev-up 批 2 拉起 minio + `wait_healthy 60`；init 依赖 service_healthy | `infra.yml:276`、`scripts/dev-up.sh:113-122` |
| **F-j** | **k8s 侧无 MinIO**：`charts/` grep minio 零命中 ⇒ prod 对象存储部署未定义（A1 依据） | grep 输出为空（本轮实测） |
| **F-k** | **环境基线（2026-10-03 计划期实测）**：全栈 Up（E2E-26 收口后遗留运行），minio/minio-init 正常，`check_minio_health` 4/4 PASS；`.devmode-session` 无人登记 | `docker ps` + 守卫输出 + 锁文件不存在 |

### 0.2 开工复核清单（第一天执行，防止任务书事实表过期）

| # | 复核项 | 通过标准 |
|---|--------|----------|
| 1 | 登记 `.devmode-session` → 按 RUNBOOK §2.1 带 `--env-file .env.local --profile dev` 起栈（若已被停）→ Nacos `count:6` → 全 healthy；apisix-seed / db-migrate / minio-init 均 Exited(0) | `docker ps` 输出留档 |
| 2 | F-a 复核：`mc anonymous get` 仍 = `download`；宿主匿名 GET 新对象仍 200 | 两探针输出 |
| 3 | F-b 复核：psql 回读 `avatar_url` / `messages.content` 仍为绝对地址（修复前基线照） | 查询结果留档 |
| 4 | F-e 复核：25MiB 对象是否仍在 + 当前上传上限运行时行为（`file` kind 实发 21MB → 期望 413） | 两向输出 |
| 5 | F-h 复核：两守卫本地复跑全绿（`check_minio_health.sh` / `smoke_upload_minio.sh`） | rc=0 |
| 6 | 内存余量基线（19 容器 ≈6G 贴顶前科，`.wslconfig` 8GB） | `docker stats` 余量 ≥1G |

---

## 1. 范围

**做**：
- 组 A（存储基座与健康）：init 幂等与策略落地、健康契约与 CI 接线、版本/镜像事实回读、MinIO 宕机降级两向、生产部署缺口记录
- 组 B（上传链路与对象契约）：头像上传存储层闭环（对象 + key 契约 + 落库）、服务端上限/mime/kind 负向、上传对象**发起者视角双视角**可达
- 组 C（匿名读权限与鉴权边界）：桶级匿名读范围实测、**M2 裁定**（匿名范围 + `:9000` 暴露处置）、voice 反代对照组（F-113 回归基线）、对象 URL 鉴权路径现状
- 组 D（URL 契约与生命周期）：**M1 裁定 F-116 修复范围与形态** → TDD 落地、fileSourceURL 耦合回归、孤儿对象生命周期处置、DB 持久化形态对照、浏览器发起者视角验收、回归钉与守卫接线

**不做（明确划出边界）**：
- **其他文件类型接入**（roadmap 边界列）：video/file 类附件的前端接入、新对象类型扩展——不属本阶段
- **多桶/纠删码/生命周期策略（ILM）等存储架构改造**——除非 M2 裁定分桶，否则单桶现状只验证不重构；架构级改动先 ADR
- **prod / k8s 对象存储部署**（charts 无 MinIO 清单，A1）——本阶段只跑 dev compose，发现 charts 漂移只记账
- **MinIO 凭证治理 / 密钥轮换 / K8s Secret 模板**——README 已声明留待后续 Sprint
- **E2E-11 已验的 UI 交互**（前端 2MB 拦截、上传按钮、提示文案）——只做存储层与发起者视角渲染验收
- **Loki/SkyWalking 观测面**（E2E-21/26 已收口）——本阶段不接观测，发现记账

---

## 2. 测试点清单（20 个）

> 判定分级：`[A]` 自动/脚本断言 · `[V]` 视觉/IAB 实测 · `[M]` 需裁定（执行期决策点，见 §4）。**对象 URL 类断言一律"发起者视角可达"**（ADR 后果段 + RUNBOOK §2.4 坑表）：宿主 curl 200 **不算**通过，必须补 web 容器视角 / 浏览器实际加载（`naturalWidth > 0`），负向对照按点内标注。

### 组 A：存储基座与健康（F-g / F-h / F-i / F-j）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 1 | [A] | **minio-init 幂等与匿名策略落地**：init Exited(0)；重跑 init 走 `mc ls \|\| mc mb` 幂等分支仍 0；`mc anonymous get` 返回 `download` | 两次 init 退出码 + 策略输出 |
| 2 | [A] | **健康契约与 CI 接线**（F-h）：`check_minio_health.sh` 4/4 + `smoke_upload_minio.sh` 4/4 本地复跑；**两守卫接入 e2e-guards**（或给出不接的理由并记账） | rc=0 双守卫 + workflow 步骤行号回读 |
| 3 | [A] | **镜像与版本事实回读**（F-g）：`mc admin info` 版本回读 = 任务书基线或说明变化；`:latest` 未 pin digest 现状定性（`check_docker_digests` 覆盖范围核对） | 版本输出 + 守卫覆盖结论 |
| 4 | [A] | **MinIO 宕机降级两向**（F-i）：停 minio → avatar 上传 503（storage nil/put err）+ BFF health 依赖翻转；恢复 → 200 恢复 | 两向状态码 + health 时间戳 |
| 5 | [A] | **生产部署缺口记录**（F-j/A1）：charts grep 零命中复核 → 结论记入 report（dev-only 边界声明）；若属应建未建则记账 | grep 输出 + 账本行（如有） |

### 组 B：上传链路与对象契约（A4 / F-d / F-e）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 6 | [A] | **头像上传存储层闭环**：上传 → `mc ls` 出现新对象且 key 命中 `avatars/<uid>-<sha8>.<ext>` 契约 → DB `avatar_url` 同步更新（E2E-11 #5 的存储层深化，非重复） | 对象清单 + key 正则 + psql 回读 |
| 7 | [A] | **服务端上限负向双查**（F-e）：avatar >2MB → **413**（F-85 服务端修复复验）；`kind=file` 实发 21MB → **413**；存量 25MiB 对象定性（历史上传 or 绕过——若可复现绕过即真缺陷进修复队列） | 三组状态码 + 定性结论 |
| 8 | [A] | **mime/kind 白名单负向**：`kind=image` 传 `application/octet-stream` → 415；`/uploads/malicious` → 400；缺 X-User-Id → 401 | 三组状态码 |
| 9 | [A] | **上传对象发起者视角可达**（F-113 教训）：新对象 ① 宿主 curl 200 ② `docker exec web-bff` 容器视角 200 ③ web 容器视角（若与 ② 不同网域）——**三视角全 200** 才 PASS | 三视角 curl 输出对照 |

### 组 C：匿名读权限与鉴权边界（F-a / F-f —— roadmap 核心范围）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 10 | [A] | **匿名读范围实测**：`avatars/` `uploads/` `voice/` 三前缀各取 1 对象，宿主 **无鉴权** GET 均 200（桶级 download 范围事实留档） | 三前缀 curl 状态码 |
| 11 | [M] | **M2：匿名读范围与 `:9000` 暴露处置**：F-a（voice 同桶匿名可绕 JWT 直读）+ F-f（0.0.0.0 暴露）→ ① dev 保持现状 + 文档化边界 ② voice 分桶去匿名（改 init + handler，动 A3） ③ 映射改 `127.0.0.1` 限定（D-38 同型）——用户拍板 | 决策登记 D-4x + 对应落地/文档化 |
| 12 | [A] | **voice 反代对照组**（F-113 回归基线，M1 的先例锚点）：`GET /api/v1/voice/audio/:key` 走网关带 JWT 200 + Content-Type 回读；key 含 `../` → 400/404 双层防御；不存在 key → **404 非 200** | 三向状态码 + header |
| 13 | [A] | **对象 URL 鉴权路径现状记录**：现状 avatar/uploads 绝对地址直连 `:9000` **不经网关**（安全面事实）；对照 voice 走网关——两路径并列留档 | 两路径 curl 对照 + report 记录 |

### 组 D：URL 契约（F-116）与生命周期（F-b / F-c / F-d）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 14 | [M] | **M1：F-116 修复范围与形态**：范围备选 ① 仅 avatar 反代 ② avatar+uploads ③ 全量（含 chat file 持久化数据与前端）；形态按 ADR"每类一端点"或通用对象端点；存量数据 回填 or 惰性归一 → 用户拍板 | 决策登记 D-4x + 范围写入 report |
| 15 | [A] | **F-116 修复 TDD（M1 裁定后）**：RED——契约断言响应与新落库 URL **不含** `localhost:9000`/PublicBaseURL（负向对照：修前现状必红）→ GREEN——反代端点 + 前端适配 + 存量处置按裁定 | RED/GREEN 两段 commit + 运行时回读 |
| 16 | [A] | **fileSourceURL 耦合回归**（F-c）：M1 修复后 chat 附件 LLM 抓取链不断——`fileSourceURL` 前缀重写/allowlist 路径按裁定适配并回归；若 M1 未触 chat file，验证现状耦合点 + 记账说明 | 附件消息端到端（发文件→AI 读到附件）或负向记录 |
| 17 | [A] | **孤儿对象生命周期**（F-d）：复现"头像更新不删旧"（更新前后对象数 +1）→ 处置：**修复**（UpdateMe 成功后删旧，TDD；须有回滚=dev 数据可重建）**或**记账写明不修理由（缓存旧 URL 兼容性）——执行者择优，存疑升级 | 对象数前后对照 + 处置结论 |
| 18 | [A] | **DB 持久化形态对照**（F-b）：修复后新上传落库为**网关相对路径**；存量绝对地址行按 M1 裁定处置（回填 SQL or 归一读取）——新旧两行对照 | psql 新旧对照 + 处置留档 |
| 19 | [V] | **浏览器发起者视角验收**：IAB 头像 `<img>` **加载完成断言**（`naturalWidth > 0`，非 URL 字符串相等）+ 上传→刷新→头像仍可见 + 匿名对象页面直开 200 | 截图 `27-*.png` 且被查看 + DOM 断言 |
| 20 | [A] | **回归钉与守卫接线收口**：`emotion-echo-web/e2e/object-storage.spec.ts` 收口跑绿（≥1 用例）；minio 守卫接线状态复核（#2 结果）；全量 `pnpm playwright test` 收口 | spec 运行输出 + workflow 回读 |

---

## 3. TDD 循环划分（RED→GREEN→REFACTOR）

> 运行时验收类测试点（#1、#3-#13、#16-#20）是验收断言，不作 TDD 对象；**守卫/修复/配置改动**必须先红后绿：

| 循环 | RED（先写失败的测试） | GREEN（最小实现） |
|------|----------------------|-------------------|
| C1 | F-116 avatar 反代契约测试：断言 `POST /user/avatar` 响应 URL 为网关相对路径、**不含** PublicBaseURL（修前现状必红）+ 前端 `getAvatarPath` 适配测试 → FAIL | avatar 反代端点 + 前端适配（范围按 M1 裁定） |
| C2 | （M1 裁定含 uploads 时）uploads 响应 URL 相同断言 → FAIL；未含则本循环改为"uploads 现状固化"负向守卫（断言现状绝对地址 + 注记 F-116 范围外） | upload_handler URL 逻辑改造 |
| C3 | 结构守卫：匿名策略/端口映射按 **M2 裁定**断言（如 compose `9000` 绑定 `127.0.0.1` / init 含分桶命令）→ FAIL（现状与裁定不符时红） | compose/init 改造或文档化条款 |
| C4 | （若 #17 裁定修复）头像更新成功后旧对象被删的契约测试（fakeStorage 断言 RemoveObject 调用参数）→ FAIL（现状无调用） | avatar_handler 成功分支删旧逻辑 |

---

## 4. 执行期 [M] 决策点

| # | 决策 | 背景 | 备选 |
|---|------|------|------|
| M1 | **F-116 修复范围与形态**（#14/#15/#16/#18） | ADR 已裁方向（相对+反代），但范围牵动：chat file 持久化数据（F-b）、fileSourceURL 耦合（F-c）、roadmap 边界"其他文件类型接入" | 范围：① 仅 avatar ② avatar+uploads ③ 全量含存量数据；形态：每类一端点（ADR 字面）vs 通用对象端点；存量：回填 vs 惰性归一 |
| M2 | **匿名读范围与 `:9000` 暴露处置**（#11） | F-a（voice 同桶匿名绕 JWT）+ F-f（0.0.0.0 暴露）——安全性与 dev 便利的取舍，无客观对错 | ① 保持+文档化 ② voice 分桶去匿名（动 A3 假设，连带 init/handler） ③ `127.0.0.1` 限定映射（D-38 同型） |

> 决议产生后登记 [decisions.md](../../decisions.md)（**D-40 起**；D-37/38/39 为 E2E-26 已占用）；涉及存储架构语义的同步 `docs/architecture/decisions.md`（**决策 39 起**）。

---

## 5. 收口门槛

1. 20/20 测试点四值判定（PASS/FAIL/BLOCKED/N/A），**BLOCKED ≤ 1/3 不得判 done**；`N/A` 必须附"为什么不适用"证据
2. 名下账本 **E2E-F-116 按 M1 裁定闭环**（翻 ✅）或写明"为何仍挂"（存在未解决条目 ⇒ 只能 `partial`）；执行期新发现按 RUNBOOK §5 契约登记（连续编号）
3. `e2e_stage_audit.py --all` 0 FAIL（plan 已用「测试点清单」标题 + 整数编号首列，A3 可解析——吸取 E2E-25 F-180 / E2E-26 经验）
4. Playwright 回归钉落地：`emotion-echo-web/e2e/object-storage.spec.ts`（收口时跑过且绿）
5. M1/M2 裁决登记 decisions.md（D-40/41 起）
6. RUNBOOK 涉及处更新（命令速查补 MinIO 对象/策略探针 + 守卫接线状态）
7. report.md 按 §10 模板 + §7 收口 11 项全过 + 第二方核对（§13.3）后才可判 done

---

## 6. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 环境基线随时可能被其他会话停掉（F-k 是遗留运行态） | 开工复核 #1 先恢复 + 登记 `.devmode-session` |
| M1/M2 未裁定阻塞组 C/D 大面积 BLOCKED | BLOCKED > 1/3 前升级；组 A/B 不依赖裁定先行；M 点集中在 #11/#14 不超总 1/3 |
| F-116 修复触及前端 + chat 附件链（跨模块扩散） | 范围由 M1 圈定；`useAIStreamHandler.ts` 属 Lane E 独占列可触（并行协议 §二），但改动必须带 #16 回归 |
| #17 删旧对象属准破坏性操作 | dev 数据可重建（回滚路径存在）；无回滚不删（RUNBOOK §11）；存疑升级用户 |
| 25MiB 存量对象若定性为绕过 = 真缺陷 | 进修复队列走 TDD；仅历史遗留则记账 F-1xx |
| `:latest` 镜像在重建时静默升级引入行为漂移 | #3 版本回读留基线；重建容器前核对版本（启动铁律 §三.资源1 镜像时间戳） |

---

## 7. 引用

- 账本：[discovered-unresolved.md](../../discovered-unresolved.md)（2026-10-03 对账：归属 E2E-27 条目 = **1，E2E-F-116**）
- ADR：[adr-2026-09-client-object-url-bff-proxy](../../../architecture/adr/adr-2026-09-client-object-url-bff-proxy.md)（F-116 修复方向的裁定源）
- 决策：[decisions.md](../../decisions.md)（编号续 D-40 起）；[docs/architecture/decisions.md](../../../architecture/decisions.md)（决策 39 起）
- 历史阶段：[E2E-11 plan](../e2e-11-my-space/plan.md)（头像 UI 链路已验）、[E2E-16 report](../e2e-16-multimodal/report.md)（voice 落 MinIO + 反代）、Sprint 1 PR-4a/4b/4c（`deploy/minio/README.md`）
- 工具：`scripts/check_minio_health.sh`（4 契约）、`scripts/smoke_upload_minio.sh`（4 契约，§契约 8）
- 官方文档检索记录：min.io docs / minio/mc GitHub 文档本轮均不可达（404），匿名语义以运行时 `mc anonymous get` 实测为准（见 §0 smoke 段）
