# ADR · 2026-09 · Nuxt 渲染模式切换到 SSR

> **本文档是追认记录（retroactive ADR）**：`ssr: false → true` 的切换实际发生在 E2E-01 阶段（commit `6c91525`，8 文件 +113/-28），当时**没有决策记录**——既不在 `docs/architecture/adr/`，也不在 `decisions.md`。
> 该缺口由 2026-09-18 对 E2E-01~06 的独立审查发现并登记为账本 `E2E-F-52`，`R-02 #6` 要求补齐。本 ADR 记录"已发生的决策"，不改变实现。
>
> **决策状态**：✅ **Accepted**（2026-09-17 实施；2026-09-18 追认补档）
> **登记位置**：[decisions.md](../decisions.md) 决策 24

---

## 一、背景

E2E-01（登录会话持久化）阶段实测发现：SPA 模式（`ssr: false`）下存在**会话恢复时序缺陷**——

1. 登录成功后前端写 cookie（`useCookie`）是**异步**的；
2. 紧随其后的 `fetchUserInfo` 在 cookie 尚未落盘/可读时发出 → BFF 侧拿到空凭据 → 401；
3. 表现是"登录成功后立刻被踢回 /login"或刷新页面时闪现登录页。

根因是 SPA 模式下**服务端不参与首屏判定**：鉴权状态只能等客户端 JS 起来、cookie 可读之后才知道，首屏渲染与鉴权之间存在窗口期。

## 二、决策

**采用 Nuxt SSR（`ssr: true`）**，让服务端在渲染首屏前读取 cookie 并决定鉴权状态，消除上述窗口期。

**证据**（`emotion-echo-web/nuxt.config.ts:8`）：

```ts
  ssr: true,
```

配套改动（同一 commit `6c91525`，均为"让客户端专属 API 不在服务端执行"的必要修正）：

| 文件 | 改动 | 原因 |
|------|------|------|
| `app/utils/db.ts` | Dexie 懒初始化 | IndexedDB 在 SSR 不可用 |
| `app/utils/messageCache.ts` | 全部函数改 `await getDb()` | 同上 |
| `app/composables/useTTSPlayer.ts` | pcm-player 懒 import | `AudioContext` 在 SSR 不可用 |
| `app/components/digital-human/DigitalHuman.vue` | 重命名为 `.client.vue` + `defineAsyncComponent`/`ClientOnly` | Three.js WebGL 在 SSR 不可用 |
| `app/middleware/auth.global.ts` | 新增 `import.meta.server` 分支 | SSR 侧读 cookie header 判定；客户端侧兜底 |

## 三、备选方案与为何不选

| 方案 | 为何不选 |
|------|---------|
| 保持 SPA，改为"登录后 await cookie 写入再发 fetchUserInfo" | 只能修"登录后立刻 401"这一条路径；**刷新页面**时服务端仍不参与，首屏闪现登录页无法消除（鉴权判定天然滞后到 JS 启动之后） |
| 保持 SPA，把 token 放内存 + 手写等待逻辑 | 与既有 `useCookie`/HttpOnly 约定冲突，且把时序耦合散布到调用点；P0-R2-1 明确要求 token 不落 localStorage |

## 四、后果

**正面**：首屏即知鉴权状态，无闪烁窗口；刷新/冷启动直接由服务端判定，`auth.global.ts:57` 的 `import.meta.server` 分支承担该职责。

**代价（必须遵守的约定）**：

- **任何依赖浏览器专属 API 的代码必须做 SSR 隔离**：Dexie/IndexedDB、`AudioContext`、WebGL 一律走懒初始化 / 懒 import / `ClientOnly` / `.client.vue`。新增此类组件时若忘记隔离，症状是 SSR 阶段抛错而非静默——相对容易发现，但会直接 500。
- **Playwright 需要 hydration 感知的等待**：SSR 下元素可见 ≠ 事件处理器已就绪，点击可能不触发（E2E-01 的 3 个 spec 曾因此失败，已按此修正）。规范化为：等待可交互信号而非仅 `visible`。
- **构建产物形态变化**：`pnpm build` 产出含 `index.html`；E2E-04 的产物 smoke 断言依赖这一点。

**风险**：SSR 使"客户端专属代码"成为一类持续存在的编码约束。缓解方式是上面的约定 + 代码评审检查项；若未来发现隔离成本持续高于收益，应重新评估本项目是否真的需要 SSR（届时本 ADR 需被 supersede，而不是静默改回）。

## 五、与其它文档的关系

- 本次追认同时更正了 **7+ 处仍写"项目是 SPA"的失效文档**（含 `stages/e2e-04-frontend-engineering/plan.md` 的「~~不引入 SSR~~」），见 `E2E-F-52`。
- 渲染模式属"架构关键词"，此后该类改动由 `scripts/check_adr_gate.sh` 检查是否附带 ADR + `decisions.md` 变更（R-03 #4）。
