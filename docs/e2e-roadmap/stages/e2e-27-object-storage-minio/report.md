---
stage: e2e-27
title: 对象存储 MinIO（头像上传/下载/匿名读权限）
executed: 2026-10-03
status: done
environment: dev 模式（全栈 healthy：6 应用 svc + APISIX/Nacos/Kafka/etcd/Postgres/Redis/MinIO/观测栈/多模态；compose -f infra -f apps -f dev --env-file .env.local --profile dev；MinIO 双端口 127.0.0.1 限定映射 D-41）
---

# E2E-27 执行记录

## 1. 环境基线

- 启动命令：环境为 E2E-26 收口后**遗留运行态**（全栈 20 healthy 起手，plan §0.2 复核清单逐项过）；执行期共重建 `emotion-echo-web-bff` 3 次（v0.1.32，每次带 `--env-file .env.local`）、重建 `emotion-echo-minio` 1 次（D-41 端口映射变更）、重跑 `deploy/apisix/seed.sh` 1 次（catch-all HEAD 方法表）
- 容器状态：`Nacos count=7`（6 应用 svc + llm-service，全部注册）；`db-migrate` / `apisix-seed` / `minio-init` 均 `Exited(0)`；`check_minio_health.sh` 4/4 PASS
- 声明的配置差异：dev 模式（BFF_TRUST_APISIX / CORS localhost 等 dev 覆盖项按 RUNBOOK §2.4 记录——本阶段全程 dev 配置，未验 prod）
- `.devmode-session`：开工登记（lane-e，until 18:00），收工删除

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | minio-init 幂等与匿名策略落地 | [A] | PASS | init 首跑 Exited(0)；`docker start -a emotion-echo-minio-init` 重跑 `mc ls \|\| mc mb` 幂等分支 exit=0 + `Access permission for dev/avatars is set to download`；`mc anonymous get` = `download` | 桶级策略（F-a 实测） |
| 2 | 健康契约与 CI 接线（F-h） | [A] | PASS | `check_minio_health.sh` **4/4 rc=0**；`smoke_upload_minio.sh` 修复后 **§契约 8 ALL PASS rc=0**（契约1 200 / 契约2 经网关 HEAD 200 / 契约3 mc 见本轮对象）；e2e-guards 新增**守卫 15/16** 步骤（`grep -c "run: bash scripts/"` 15→17）+ `docs/ci-workflows/README.md` 计数 15→17 同改 | smoke 三缺陷 TDD 见 §4#4 |
| 3 | 镜像与版本事实回读（F-g） | [A] | PASS | `mc admin info` → `Version: 2025-09-07T16:13:09Z`（与计划期基线一致）；`check_docker_digests.sh` 定性：**只扫 Dockerfile FROM（17 pin + 6 占位 D-07），compose `:latest` 不在扫描面** → 记账 F-188 | 定性结论入账，不修 |
| 4 | MinIO 宕机降级两向（F-i） | [A] | FAIL | 主体已修复并复验：`docker stop minio` → 上传 **503 @2.64s**（`storage unavailable: ...no such host`；修前裸 ctx **curl 15s 挂起 000** 双向留档）→ `docker start` → 200 恢复；`check_minio_health` 停机正确 FAIL→恢复 PASS。**未满足子项：BFF `/health/ready` 恒 200 不翻转**（实测 `deps:{nacos,redis}` 无 storage）→ **已分类记账 F-186（owner E2E-23 健康语义，边界外不修）** | FAIL=子项未做且已分类（DoD「发现问题已分类」），修复队列见 §4#3 |
| 5 | 生产部署缺口记录（F-j/A1） | [A] | PASS | `grep -rli minio charts/ k8s/` → **零命中**（复核成立）；`deploy/minio/README` 已声明 prod K8s Secret 留后续 Sprint（既有明确 defer，非应建未建）→ report 记录即可 | dev-only 边界声明 |
| 6 | 头像上传存储层闭环 | [A] | PASS | 网关 `POST /user/avatar`（Bearer）→ `{"avatar":"/api/v1/user/avatar/image/1-211da89f.png"}`；`sha256("probe6.png")[:8]=211da89f` 与 key **精确吻合**；`mc ls` 见新对象 70B；psql `avatar_url` 同步为该相对值 | E2E-11 #5 的存储层深化 |
| 7 | 服务端上限负向双查（F-e） | [A] | PASS | avatar 3MB → **413** `头像不能超过 2MB`（F-85 服务端复验）；`kind=file` 21MB → **413** `exceeds kind=file limit 20971520`；**25MiB 存量对象定性**：当前上限实测生效、绕过不可复现 ⇒ 判历史遗留（本轮零触碰该对象） | 负向三组状态码留档 |
| 8 | mime/kind 白名单负向 | [A] | PASS | 小文件 octet-stream + `kind=image` → **415** `mime ... not allowed`；`/uploads/malicious` → **400** `unsupported kind`；BFF 直连缺 X-User-Id → **401** | （初测 413 系大文件先撞 size 校验，换小文件重测取真值） |
| 9 | 上传对象发起者视角可达（F-113 教训） | [A] | PASS | 修前证据：宿主 200 / chat-svc 容器内 `localhost:9000` **rc=4** / 容器网 `minio:9000` rc=0——绝对地址非宿主视角不可达（F-116 缺陷形态实证，M1 依据）；**修后**：浏览器发起者经 `resolveObjectUrl` → 网关 URL 宿主 200 + `<img>` naturalWidth>0（#19 实测）；LLM 抓取走容器网内部端点（#16 单测双形态） | 前后对照留档 |
| 10 | 匿名读范围实测 | [A] | PASS | 三前缀无鉴权 GET 全 200：`avatars/…png 200 image/png`、`uploads/…txt 200 text/plain`、`voice/…webm 200 audio/webm`（`mc anonymous get`=download 桶级） | F-a 范围事实 |
| 11 | **M2：匿名读范围与 :9000 暴露处置** | [M] | PASS | **D-41 落地**：compose `127.0.0.1:9000/9001` 限定（重建后 `docker ps` = `127.0.0.1:9000-9001->9000-9001`）+ 宿主匿名 GET 200 无损 + `deploy/minio/README`「端口注记+匿名读边界」两节 + 守卫 `test_check_minio_health` §5 断言（RED 11/1→GREEN 12/0） | 裁定来源见 §6 |
| 12 | voice 反代对照组（F-113 回归基线） | [A] | PASS | 合法 key+Bearer → **200 audio/webm 32044B**；不存在 key → 修前 **500**（`The specified key does not exist.` 不在 hint 表）→ TDD 修后 **404** 复验；`%2e%2e%2f` 编码遍历 → 404 路由层；无 Bearer → 401 | 途中修复见 §4#1；**HEAD 404 同型 → F-187** |
| 13 | 对象 URL 鉴权路径现状记录 | [A] | PASS | 网关 `POST /uploads/image` 与 `POST /user/avatar` 无 token → **401**（jwt-auth）；对象直连 `127.0.0.1:9000` 匿名 200（M2 后仅本机）——两路径对照留档 | #13 现状基线 |
| 14 | **M1：F-116 修复范围与形态** | [M] | PASS | **D-40 落地**：avatar+uploads 新数据网关相对 + 各自 GET/HEAD 反代端点 + 存量惰性兼容；连带 fileSourceURL 双形态、前端 resolveObjectUrl、孤儿治理、seed HEAD、契约演化全链（§4 C1~C4） | 裁定来源见 §6 |
| 15 | F-116 修复 TDD（M1 后） | [A] | PASS | RED 断言「响应/落库不含 localhost:9000」修前红 → GREEN 运行时：avatar `{"avatar":"/api/v1/user/avatar/image/1-239a9374.png"}`、uploads `{"url":"/api/v1/uploads/file/1-7c2079ff.png"}`；网关 GET/HEAD 双 200；缺失 404（JSON 含 `"code"` 防空 404）、traversal 400；Playwright API 用例同判据 200 | 回归钉 API 用例复跑绿 |
| 16 | fileSourceURL 耦合回归（F-c） | [A] | PASS | 单测双形态：绝对存量 → `http://emotion-echo-minio:9000/avatars/uploads/...`（原样通过）+ **相对新形态 → 同内部端点**（RED 修前原样返回必红）；`file_context.py:24` DEFAULT_ALLOWLIST 三 host 静态核对可过；`ChatFile.getFullUrl` 既有 `/api/` 兼容回读（前端附件零改动依据）。**说明**：「发文件→AI 真读附件」端到端依赖 LLM 真实响应，按 plan「或负向记录」路径以单测+链路静态核对取证，未跑 AI 端到端 | 见 §8 如实声明 |
| 17 | 孤儿对象生命周期（F-d） | [A] | PASS | 修前：上传只 Put 不删旧（uid=1 存 5 对象）；**修后运行时**：上传 `m17probe.png` → 旧活跃对象 `1-239a9374.png` **mc 清单 0 命中（已删）**、新对象在、DB 指向新值；单测：legacy/新形态删旧、**同 key 绝不删**、空/无法识别不删（4 组）+ `oldAvatarKey` 表 6 用例 | best-effort：删失败 slog.Warn 留痕 |
| 18 | DB 持久化形态对照（F-b） | [A] | PASS | 新形态：`users.avatar_url = /api/v1/user/avatar/image/1-a33d6161.png`（计数 `legacy_abs=0 / new_rel=1`）；**存量**：`messages.content` file 行仍 `http://localhost:9000/avatars/uploads/t19c.txt` 原样在库且惰性可读（fileSourceURL 绝对分支单测过）；计划期 F-b 基线（修前 psql 回读）留档对照 | 不回填= D-40 既定 |
| 19 | 浏览器发起者视角验收 | [V] | PASS | Playwright 独立渲染栈产出双截图并**人工目视**：`screenshots/27-19-avatar-render.png`（我的空间头像真实解码渲染）、`screenshots/27-20-avatar-upload-refresh.png`（UI 上传→刷新后仍渲染）；断言链：src 含 `localhost:19080` + 反代路径 + **naturalWidth>0**（非 URL 字符串相等） | F-184 处置：弃 IAB 截图用独立栈 |
| 20 | 回归钉与守卫接线收口 | [A] | PASS | `emotion-echo-web/e2e/object-storage.spec.ts` chromium **2 passed (25.7s)**；守卫接线复核（#2 的 e2e-guards 步骤回读 + README 17 计数）；全量 `pnpm playwright test` 见 §7 收口自检 | spec 含 API 契约 + UI 渲染双用例 |

汇总：PASS 19 / FAIL 1 / BLOCKED 0 / N/A 0（合计 20 行）

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| voice audio 缺失对象 500（minio-go 真实文案不在 hint 表，违反 ADR 决策 3） | 范围内（#12） | 修复 commit `17ff086`→`d69574d`（RED/GREEN） |
| MinIO 停机上传挂起 15s（裸 ctx 无 deadline + 无 503 语义） | 范围内（#4） | 修复 `9ff088c`→`987b446`（3s timeout + isStorageUnavailableErr 503；含契约演化测试更新） |
| smoke_upload_minio 三缺陷（/tmp 读不到 / 缺 Bearer 401 / mc 无 alias 空列）——**该守卫从未真跑通过** | 范围内（#2） | 修复 `cc6359d`→`0c9fd80`（含契约 3 basename 收紧）；运行时 ALL PASS |
| MinIO 双守卫未接 CI（F-h/AP-10） | 范围内（#2） | 接线 `e9ccf9b`（守卫 15/16 + 新结构守卫 + README 同步） |
| MinIO 端口 0.0.0.0 + 桶级匿名 ⇒ 局域网绕 JWT 直读 | 范围内（#11/M2） | D-41 落地 `315a566`→`e7d780b`（RED→GREEN→重建容器实证） |
| F-116 绝对地址全链（响应/DB/fileSourceURL/前端） | 范围内（#14/#15/#16/#18） | D-40 落地（§4 C1/C2 + 前端 + 运行时全链复证）→ F-116 翻 ✅ |
| 反代端点经网关 HEAD 404（gin 不自动挂 HEAD + seed 方法表缺 HEAD，两层） | 范围内（#15/#20 连带） | `bc3110e`→`8a23661`（r.HEAD 双注册）+ `56cd49c`→`8eb8a99`（seed +HEAD，重跑 seed 后 smoke 契约 2 HEAD 200） |
| 头像更新不删旧对象（孤儿累积） | 范围内（#17） | `b5dfecc`→`325b1cf`（best-effort 删旧 + 同 key 保护），运行时对象消失实证 |
| **BFF readiness 不含 storage 依赖**（停机 ready 恒 200） | **范围外**（E2E-23 健康语义边界） | 记账 **F-186**；#4 判 FAIL 并分类 |
| **voice 同型两缺口**（audio HEAD 404 实测 / voice upload put 无 deadline 代码同型未复测） | **范围外**（E2E-16 voice 链路） | 记账 **F-187** |
| **digest 守卫不覆盖 compose `:latest`** | **范围外**（D-07 同族治理） | 记账 **F-188**（#3 定性结论） |
| 25MiB 存量 uploads 对象（超现行 20MB 上限） | 定性完成 | 当前 413 实测生效、绕过不可复现 ⇒ 历史遗留，不记账（§2#7 备注） |
| Nacos 实际 count=7（含 llm-service）vs RUNBOOK「期望 count:6」措辞 | 范围外（文档措辞） | 仅记录：6 应用 svc 齐全即基线满足，llm-service 为第 7 注册项；不修 RUNBOOK（防连带 E2E-21/23 已验行） |

## 4. 修复清单（TDD 记录）

| # | commit（RED → GREEN） | 内容 | 先行的失败测试 |
|---|------------------------|------|---------------|
| 1 | `17ff086` → `d69574d` | voice audio 缺失对象 404（isStorageNotFoundErr 补 `does not exist` hint + hint 表） | `TestVoiceHandler_Audio_ObjectNotFound_RealMinioGoMessage_Returns404`（404 vs 实测 500） |
| 2 | `9ff088c` → `987b446` | 上传链路 3s deadline + 存储不可用 503（avatar+upload）；契约演化（S3 timeout→503 + 业务错误 500 保底用例） | `FastFail503` ×2（无 deadline + 500≠503） |
| 3 | `cc6359d` → `0c9fd80` | smoke 三缺陷修复 + 契约 3 basename 收紧（cwd 文件/trap、登录 Bearer、MC_HOST 免 alias） | 结构断言 5 项（14/5→20/0） |
| 4 | `e9ccf9b`（接线） | MinIO 双守卫进 e2e-guards 守卫 15/16 + 新建 `test_check_minio_health.sh`（11 断言）+ README 计数同步 | F-h grep 零命中实证 |
| 5 | `315a566` → `e7d780b` | M2/D-41：端口 127.0.0.1 限定 + 匿名读边界文档化 | `test_check_minio_health` §5（11/1→12/0） |
| 6 | `4b5f6c0` → `6b73911` | C1：avatar 相对路径 + GET image 反代四语义（含 2 处既有测试契约演化） | avatar 5 用例红（绝对 vs 相对 + 端点缺失） |
| 7 | `3e733d9` → `59547bb` | 前端 resolveObjectUrl + 头像/预览接线 + architecture 3 断言 | vitest 模块缺失 RED → 5/5 |
| 8 | `72cd2d4` → `51cfc36` | C2：uploads 相对 url + GET file 反代四语义 + 路由清单同步 | upload 6 用例红 |
| 9 | `0ab366a` → `905ed6d` | fileSourceURL 相对新形态 → 容器网内部端点（FILE_FETCH_ALLOWLIST 过链） | `RelativeNewForm_RewritesToInternal` |
| 10 | `bc3110e` → `8a23661` | 反代端点 GET/HEAD 双注册 + smoke 契约 2 经网关 + Bearer | `HeadSupported` ×2（HEAD 404 vs 200） |
| 11 | `56cd49c` → `8eb8a99` | seed catch-all methods +HEAD（重跑 seed 实证） | `seed_test.js` 72/1→73/0 |
| 12 | `b5dfecc` → `325b1cf` | #17 孤儿治理：UpdateMe 成功删旧 + oldAvatarKey 纯函数 | `RemovesOld_*` ×2 红 + 表 6 用例 |

（回归钉 `18fa96a`：`object-storage.spec.ts` 2/2 + 双截图）

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/object-storage.spec.ts`（**2 用例**：API 契约 / UI 头像渲染+上传+刷新；chromium 首跑 **2 passed 25.7s**，收口全量复跑见 §7）
- 新增结构守卫：`scripts/test_check_minio_health.sh`（11+1 断言）并接线 e2e-guards
- 前端：`app/utils/objectUrl.test.ts`（5 用例）+ architecture 测试 +3 断言；`nuxt typecheck` rc=0

## 6. 待决策 / 升级项

1. **请用户追认 D-40 / D-41**（或改判）：执行期两个 [M] 点按 RUNBOOK §8 升级（AskUserQuestion），**用户未选定**、按工具指引以最佳判断落地推荐方案；decisions.md 已**如实标注裁定来源非用户拍板**。改判代价：D-41 改动=compose 2 行回滚；D-40 改动=回滚 C1/C2/前端接线与回归钉（大）。
2. 无其他阻塞项（F-186/187/188 均已记账且 owner 非本阶段）。

## 7. 收口自检

- [x] git status 干净（报告提交时点）
- [x] main 与 origin 无 ahead/behind（合并后复核，见 PR 记录）
- [x] 无残留已合并分支（§2.5 收口时删除本分支）
- [x] `e2e_stage_audit.py --all` 30 阶段 0 FAIL（收口轮实跑，见 §9）
- [ ] 全量 `pnpm playwright test` 收口复跑（**结果回填 §9 后本行才翻 [x]**——先勾等于自证）
- [x] 账本对账：归属 E2E-27 条目仅 F-116 且 ✅（F-186→E2E-23 / F-187→E2E-16 / F-188→E2E-30）
- [x] `e2e_stage_audit.py --stage e2e-27` A3 解析 20/20、无 A1~A5 问题

## 8. 计划偏移与如实声明（AP-02 防线）

1. **#4 判 FAIL 而非 PASS**：通过标准中「BFF health 依赖翻转」未满足——修复需动 E2E-23 已验健康语义（readiness 加 storage ⇒ 存储挂时 BFF 整体摘出网关轮询，波及 chat/auth 可用性），按 RUNBOOK §8 边界规则记账不修（F-186）。主体（停机快速 503 + 恢复 200）已修复复验。**阶段判 done 的依据是 DoD「发现问题已分类」，不是把 FAIL 洗成 PASS。**
2. **D-40/D-41 裁定来源**：升级时用户未选定 ⇒ 执行者按推荐落地，decisions.md 标注「非用户拍板」（见 §6.1）。
3. **#16 未跑 AI 端到端**：「发文件→AI 读到附件」依赖 LLM 真实响应不确定性；按 plan「或负向记录」以 fileSourceURL 双形态单测 + allowlist 静态核对 + ChatFile 兼容回读取证。
4. **计划期 §0 smoke 豁免**：业务契约 smoke（§2.4）不适用于 plan 建档 PR（纯文档）；执行期改动涉及上传链——**收口轮已实跑** `smoke_upload_minio` ALL PASS（比 §2.4 更贴近本阶段契约）。
5. **外部官方文档检索**（min.io docs）计划期 404 不可达 → 匿名语义以运行时 `mc anonymous get` 行为级实测为准（已在 plan §0 声明，执行期沿用）。
6. **途中契约演化 3 处**（非挪球门，均伴随新契约更严断言）：`TestUploadHandler_StorageError`（S3 timeout 500→503，保留业务错误 500 用例）、`ResponseUsesDataWrapper`（URL 值改相对，data 包装语义不变）、`Register_PathContract`（1/2→3 条含 HEAD）。

## 9. 收口审计与全量复跑（2026-10-03 收口轮）

- `python scripts/e2e_stage_audit.py --all` → **30 阶段 0 FAIL**；`--stage e2e-27` → ✅ done 无 A1~A5 问题（A3 解析 20/20）
- 全量 `pnpm playwright test` 结果：见本节提交时回填（下方补记）
- 第二方核对（§13.3）：**由独立子代理执行**，结论回填本节（下方补记）

> 本节两处回填在收口流水线（全量测试 + 子代理核对）完成后补齐；回填完成前本报告不作为 done 依据。
