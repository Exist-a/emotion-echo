/**
 * local-llm.vue 架构契约测试（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：12 项静态契约 —— 验证 demo 路由满足：
 *   - v0.3 §C.6 阶段一："demo 独立路由，生产 build 排除"
 *   - 协议 §二："不 import useAIStreamHandler"（stage1 禁触）
 *   - 协议 §二："不依赖 useApi / useUserStore"（避开 auth.global.ts 共享列握手）
 *   - SSR 隔离（happy-dom 失真绕道 + decision-24 §伴随约束）
 *
 * 测试形态：纯静态源扫描（readFileSync + regex），与 useTTSPlayer.phoneme.test.ts
 * 同型 —— 避开 happy-dom 对 .vue 的不真实渲染。
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const __filename = fileURLToPath(import.meta.url)
const __dirname = resolve(__filename, '..')
const PAGE_SRC = readFileSync(
  resolve(__dirname, 'local-llm.vue'),
  'utf8'
)

describe('pages/demo/local-llm.vue 路由契约（12 项）', () => {
  it('1. 必须存在 <script setup>', () => {
    expect(/<script\s+setup[^>]*>/.test(PAGE_SRC)).toBe(true)
  })

  it('2. 必须有 definePageMeta 调用', () => {
    expect(/definePageMeta\s*\(/.test(PAGE_SRC)).toBe(true)
  })

  it('3. definePageMeta 内必须含 layout: "default"（不用 nav layout）', () => {
    const m = PAGE_SRC.match(/definePageMeta\s*\(\s*\{[\s\S]*?\}\s*\)/)
    expect(m).not.toBeNull()
    expect(/layout\s*:\s*['"]default['"]/.test(m?.[0] ?? '')).toBe(true)
  })

  it('4. definePageMeta 内必须含 ssr: false（WebGPU/Worker 客户端专用）', () => {
    const m = PAGE_SRC.match(/definePageMeta\s*\(\s*\{[\s\S]*?\}\s*\)/)
    expect(m).not.toBeNull()
    expect(/ssr\s*:\s*false/.test(m?.[0] ?? '')).toBe(true)
  })

  it('5. 不能 import useAIStreamHandler（协议 §二 stage1 禁触）', () => {
    expect(
      /from\s+['"][^'"]*useAIStreamHandler['"]/.test(PAGE_SRC)
    ).toBe(false)
  })

  it('6. 不能 import useUserStore（避开 auth.global.ts 共享列握手）', () => {
    expect(
      /from\s+['"][^'"]*stores\/user['"]/.test(PAGE_SRC)
    ).toBe(false)
  })

  it('7. 不能 import useApi（与 useUserStore 同型 —— demo 不调线上 API）', () => {
    expect(
      /from\s+['"][^'"]*useApi['"]/.test(PAGE_SRC)
    ).toBe(false)
  })

  it('8. 不能静态 import @mlc-ai/web-llm（production bundle 隔离，协议 §二 白名单要求 dynamic import）', () => {
    expect(/from\s+['"]@mlc-ai\/web-llm['"]/.test(PAGE_SRC)).toBe(false)
  })

  it('9. <template> 内必须用 <ClientOnly> 包住所有 WebGPU 调用点', () => {
    const templateStart = PAGE_SRC.indexOf('<template>')
    const templateEnd = PAGE_SRC.lastIndexOf('</template>')
    expect(templateStart).toBeGreaterThan(-1)
    expect(templateEnd).toBeGreaterThan(templateStart)
    const template = PAGE_SRC.slice(templateStart, templateEnd + 1)
    expect(/<ClientOnly[\s>]/.test(template)).toBe(true)
  })

  it('10. 必须 import deviceCapability / routeDecision utils（契约落地证明）', () => {
    expect(
      /from\s+['"][^'"]*utils\/offline\/deviceCapability['"]/.test(PAGE_SRC)
    ).toBe(true)
    expect(
      /from\s+['"][^'"]*utils\/offline\/routeDecision['"]/.test(PAGE_SRC)
    ).toBe(true)
  })

  it('11. 必须含显式 "WIP" 标注（明示 T3 才接真引擎，§二 production 隔离）', () => {
    // 防止有读者误以为这是可运行的端侧推理页面
    expect(/WIP|未接入|placeholder|stub/i.test(PAGE_SRC)).toBe(true)
  })

  it('12. 不能使用 <el-*>/Element Plus 标签（决策 25 + e2e-11 契约）', () => {
    expect(/<el-[a-z]+[\s>]/i.test(PAGE_SRC)).toBe(false)
  })
})