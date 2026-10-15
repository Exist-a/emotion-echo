---
status: planned
priority: medium
owner: TBD
created: 2026-10-15
type: implementation-plan
source: 用户手动测试（2026-10-15）—— 用户原话："摄像头渲染的位置我希望也是固定定位可拖动可折叠的，我有个设计思路，摄像头可以移动位置，框上面设置一个小边框，点击后可以收起来"
depends-on: []
related-plans:
  - frontend-message-cache-offline-resilience.md（CL-05；本计划要动的 `pages/chat/conversation/[id].vue` 在 CL-05 文件面内 ⇒ 需握手）
---

# Plan — 摄像头预览：悬浮可拖动 + 可折叠

> **只描述要做的事，不含代码改动。**

## §0 上下文（写文档前的功课，按 AGENTS.md §〇）

### 0.1 现状（2026-10-15 实读代码）

| 项 | 现状 |
|---|---|
| 组件位置 | **没有独立组件** —— 摄像头预览是 `emotion-echo-web/app/pages/chat/conversation/[id].vue` 里的内联 `<video class="camera-preview">`（`:212`），`v-show="faceEmotion.isCameraOn.value"` 控制显隐 |
| 样式 | `.camera-preview`（`:906`）：`width:240px; height:180px; margin:12px auto 0; border-radius:12px` ⇒ **跟随文档流**（不是浮动层），固定在页面中部，**不能移动、不能折叠** |
| 状态文案 | 同页 `.camera-status`（`:220`）显示"当前表情识别：xxx"，也在文档流里 |
| 采集逻辑 | `useFaceEmotion`（`composables/`）提供 `isCameraOn` / `currentEmotion` / `startCamera(videoRef)`（`:299`）—— 本计划**不动采集逻辑**，只改呈现层 |

### 0.2 可复用的既有实现（关键）

**`app/components/digital-human/DigitalHuman.client.vue` 已经有一套"可拖动悬浮层"**，可直接照搬其模式：

- prop `draggable`（`:95`，默认 true）+ 根元素 `:class="{ draggable }"`（`:5`）；
- 位置状态 `const position = ref({ x: 0, y: 20 })`（`:120`）+ 内联样式 `:style="{ left: position.x + 'px', top: position.y + 'px' }"`（`:6`）；
- 拖动结束 `emit('position-change', {...})`（`:451`）—— 供父组件持久化。

⇒ **本计划应"提取共用悬浮层行为"而不是另写一套拖拽**（否则两处拖拽逻辑会漂移）。

### 0.3 相关文档 / 决策

- `AGENTS.md §七`：新功能计划落在 `docs/plans/`（本文件）；
- `docs/closure-roadmap/parallel-contract.md §1.1`：`pages/chat/conversation/[id].vue` 属 **C 轨（CL-02）文件面**，且 CL-05（D 轨）也声明要改该页 ⇒ **动手前须按 §四 握手**（谁先谁改）；
- 账本 **E2E-F-200**（摄像头「组件未就绪」TypeError 兜底）与 **CL-F-17**（数字人 VRM 初始化竞态）同属"前端初始化时序"族，本计划改同一页时**顺手核一遍**是否互相影响。

## §1 目标（对齐用户原话）

把摄像头预览从"页面内联块"改成**悬浮预览窗**：

1. **固定定位**（`position: fixed`，默认停靠右下角，避让数字人悬浮层与消息输入区）；
2. **可拖动**（按住窗体拖动，边界限制在视口内）；
3. **可折叠** —— 用户原话："**框上面设置一个小边框，点击后可以收起来**"：
   - 窗体**顶部有一条小把手/边条**（handle bar），**点击它即收起**；
   - 收起态 = 只剩那条把手（或一个小图标 + 尺寸提示），**再点一下展开**；
   - 收起**不停止摄像头采集**（只是不显示画面）—— 需在文档里写死这条语义，避免被理解成"关摄像头"。

**非目标**：不改表情识别链路（`useFaceEmotion` 的采集/上传/分析）；不改摄像头开关按钮的位置；不做多窗口/画中画（PiP）——留作后续。

## §2 待定（进入本计划时先拍板）

| # | 事项 | 说明 |
|---|---|---|
| D1 | 位置/折叠态是否**持久化**（localStorage） | 建议持久化，否则每次进页面复位，用户会觉得"白拖了" |
| D2 | 默认停靠位置 | 右下角？左下角？需与数字人悬浮层、输入框、侧边栏避让（建议：右下，数字人默认在左/中） |
| D3 | 折叠态的外观 | 一条把手条 vs 一个圆形小图标；是否显示"摄像头开/关"指示 |
| D4 | 是否抽成**共用悬浮层组件**（与 DigitalHuman 共用拖拽逻辑） | 建议抽（见 §0.2），但要评估改动面 —— DigitalHuman 属数字人面，动它需另握手 |
| D5 | 移动端/窄视口行为 | 小视口下悬浮窗可能遮挡内容；建议窄视口默认收起（并记入回归钉） |

## §3 实施骨架（进入本计划时细化）

1. **RED（先钉）**：Playwright 端到端断言三条 —— ① 开启摄像头后预览为 `position:fixed`；② 拖动后 `left/top` 变化且不越界；③ 点顶部把手后收起（高度塌缩、视频不可见）、再点展开。
   （vitest 侧可钉"折叠态不调用 stopCamera"这类状态契约。）
2. **GREEN**：最小实现 —— 抽出悬浮层行为（拖拽 + 折叠），摄像头预览接入；保留既有 `.camera-status` 的展示位置（或在悬浮窗内显示）。
3. **REFACTOR**：与 `DigitalHuman` 的拖拽逻辑收敛（若 D4 采纳）。
4. **回归钉**：上述 Playwright 用例 + 既有摄像头相关用例（`a12-camera-notify.architecture.test.ts`、E2E-F-200 的兜底用例）保持绿。

## §4 调研依据

- 实读：`emotion-echo-web/app/pages/chat/conversation/[id].vue`（`:212` markup、`:906` 样式、`:299` startCamera）、`app/components/digital-human/DigitalHuman.client.vue`（`:5/:6/:95/:120/:451` 拖拽模式）、`app/composables/useFaceEmotion*`
- 资源核验：`public/3d-models/digital-human.vrm`（26.7MB，dev 下 HTTP 200）
- 文档：`AGENTS.md §七`、`docs/closure-roadmap/parallel-contract.md §1.1/§四`、账本 E2E-F-200 / CL-F-17
- 用户原话（2026-10-15 手动测试）见本文件 front-matter `source`
