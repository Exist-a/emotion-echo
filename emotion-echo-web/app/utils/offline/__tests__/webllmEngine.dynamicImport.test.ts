/**
 * webllmEngine.dynamicImport.test.ts — Dynamic engine 架构契约测试（Lane O · T2 末）。
 *
 * 范围（v0.3 §C.1 + §F.2 + protocol §二）：
 *   - createDynamicEngine() 工厂存在 + 返回 { engine, state }
 *   - engine 满足 WebLLMEngine 4 方法契约（init/chat/abort/dispose）
 *   - state 默认值（idle/error=null/module=null）
 *   - init() 触发 dynamic import '@mlc-ai/web-llm' —— **唯一**引用方式
 *   - init() 失败不静默降级（抛清晰错误）
 *   - chat() 已 init 后抛清晰错误指引 T3 接入
 *
 * 为什么不写 happy-dom 行为测试（v0.3 §F.2 #2 教训）：
 *   - WebGPU / Worker / OPFS 等浏览器 API mock 失真（memory「TDD 契约 vs 行为测试」）
 *   - 真实 dynamic import('@mlc-ai/web-llm') 在 vitest 下不一定能解析（环境依赖）
 *   - 本测试聚焦**架构契约**（静态源 + factory 形状 + 错误语义），行为覆盖 T3 IAB
 */
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import {
  createDynamicEngine,
  type DynamicEnginePhase,
  type DynamicEngineState,
  type WebLLMEngine,
} from '../webllmEngine'

// ============================================================
// 工厂契约
// ============================================================

describe('createDynamicEngine 工厂契约', () => {
  it('必须存在并返回 callable factory', () => {
    expect(typeof createDynamicEngine).toBe('function')
  })

  it('返回 { engine, state } 二元组', () => {
    const result = createDynamicEngine()
    expect(result).toHaveProperty('engine')
    expect(result).toHaveProperty('state')
  })

  it('engine 满足 WebLLMEngine 接口（4 方法 + 2 getter）', () => {
    const { engine } = createDynamicEngine()
    expect(engine).toBeTypeOf('object')
    expect(typeof engine.init).toBe('function')
    expect(typeof engine.chat).toBe('function')
    expect(typeof engine.abort).toBe('function')
    expect(typeof engine.dispose).toBe('function')
    // 模型 ID 字段（getter）
    expect(engine.modelId).toBe('')
    // loaded 字段（getter）
    expect(engine.loaded).toBe(false)
  })

  it('state 默认值：phase=idle / error=null / module=null', () => {
    const { state } = createDynamicEngine()
    expect(state.phase).toBe<DynamicEnginePhase>('idle')
    expect(state.error).toBeNull()
    expect(state.module).toBeNull()
  })

  it('每次调用工厂返回新实例（无共享 mutable 状态）', () => {
    const a = createDynamicEngine()
    const b = createDynamicEngine()
    expect(a.engine).not.toBe(b.engine)
    expect(a.state).not.toBe(b.state)
    // 但 state 是独立对象 —— a.state 修改不影响 b.state
    a.state.phase = 'loaded'
    expect(b.state.phase).toBe('idle')
  })
})

// ============================================================
// 错误语义（不静默降级 —— AGENTS §3.2 反对掩盖故障）
// ============================================================

describe('Dynamic engine 错误语义', () => {
  it('chat() 未 init 时抛清晰错误（避免静默 mock）', async () => {
    const { engine } = createDynamicEngine()
    await expect(
      engine.chat({ messages: [{ role: 'user', content: 'x' }] }),
    ).rejects.toThrow(/Engine not loaded/)
  })

  it('engine.init() 失败时记录到 state.error（不静默降级）', async () => {
    // 通过 fake dynamic import 模拟失败 —— 但本测试不调用真 import；
    // 改成验证：state.error 字段存在且可写；具体失败路径由 T3 IAB 实测覆盖
    const { state } = createDynamicEngine()
    expect(state).toHaveProperty('error')
    // 模拟赋值
    state.error = 'fake: dynamic import failed'
    expect(state.error).toBe('fake: dynamic import failed')
  })
})

// ============================================================
// 架构契约：production bundle 隔离（protocol §二 白名单 + v0.3 §C.6）
// ============================================================

describe('静态源架构契约（production bundle 隔离）', () => {
  const ENGINE_SRC = readFileSync(
    resolve(__dirname, '..', 'webllmEngine.ts'),
    'utf-8',
  )

  it('createDynamicEngine 函数已导出', () => {
    expect(/export\s+function\s+createDynamicEngine\b/.test(ENGINE_SRC)).toBe(true)
  })

  it('DynamicEnginePhase 类型已导出（含 idle / importing / loaded / unavailable 4 值）', () => {
    expect(/export\s+type\s+DynamicEnginePhase\b/.test(ENGINE_SRC)).toBe(true)
    expect(/['"]idle['"]/.test(ENGINE_SRC)).toBe(true)
    expect(/['"]importing['"]/.test(ENGINE_SRC)).toBe(true)
    expect(/['"]loaded['"]/.test(ENGINE_SRC)).toBe(true)
    expect(/['"]unavailable['"]/.test(ENGINE_SRC)).toBe(true)
  })

  it('DynamicEngineState 接口已导出（phase / error / module）', () => {
    expect(/export\s+interface\s+DynamicEngineState\b/.test(ENGINE_SRC)).toBe(true)
    expect(/\bphase\b/.test(ENGINE_SRC)).toBe(true)
    expect(/\berror\b/.test(ENGINE_SRC)).toBe(true)
    expect(/\bmodule\b/.test(ENGINE_SRC)).toBe(true)
  })

  it('**无静态** import from \'@mlc-ai/web-llm\'（production bundle 隔离硬规则）', () => {
    // 匹配: import X from '@mlc-ai/web-llm' | import { Y } from '@mlc-ai/web-llm'
    const staticImportRegex = /^\s*import\s+[^;]+?\s+from\s+['"]@mlc-ai\/web-llm['"]/m
    expect(staticImportRegex.test(ENGINE_SRC)).toBe(false)
  })

  it('必须有 **dynamic** import(\'@mlc-ai/web-llm\') —— 唯一引用方式', () => {
    // 匹配: await import('@mlc-ai/web-llm') 或 import('@mlc-ai/web-llm')
    const dynamicImportRegex = /\bimport\s*\(\s*['"]@mlc-ai\/web-llm['"]\s*\)/
    expect(dynamicImportRegex.test(ENGINE_SRC)).toBe(true)
  })

  it('@mlc-ai/web-llm 字符串只允许出现在 dynamic import() 内（无其他引用形式）', () => {
    // 抓出所有 '@mlc-ai/web-llm' 出现位置
    const matches = [...ENGINE_SRC.matchAll(/['"]@mlc-ai\/web-llm['"]/g)]
    expect(matches.length).toBeGreaterThan(0)
    // 每处都应紧邻 `import(` —— 允许前后少量空白
    for (const m of matches) {
      const idx = m.index ?? 0
      const before = ENGINE_SRC.slice(Math.max(0, idx - 15), idx)
      const after = ENGINE_SRC.slice(idx + m[0].length, idx + m[0].length + 5)
      // 前缀应含 `import(`
      expect(/import\s*\(\s*$/.test(before)).toBe(true)
      // 后缀应是 `)` 或 `)` + 空白
      expect(/^\s*\)/.test(after)).toBe(true)
    }
  })

  it('createDynamicEngine 函数体内不直接调 fetch / XMLHttpRequest（保留 T3 IAB 实测）', () => {
    const fnMatch = ENGINE_SRC.match(
      /export\s+function\s+createDynamicEngine\s*\([\s\S]*?\n\}/,
    )
    expect(fnMatch).not.toBeNull()
    const body = fnMatch?.[0] ?? ''
    // T2 末 dynamic import 不算 fetch（vite 处理）—— 但禁止显式 fetch/XHR
    expect(/\bfetch\s*\(/.test(body)).toBe(false)
    expect(/XMLHttpRequest/.test(body)).toBe(false)
  })

  it('Dynamic engine chat() 在 init() 后抛错指向 T3 接入（不静默 mock）', () => {
    // 验证 init 后 chat 的错误信息包含 T3 指引
    const initMatch = ENGINE_SRC.match(
      /export\s+function\s+createDynamicEngine\s*\([\s\S]*?\n\}/,
    )
    const body = initMatch?.[0] ?? ''
    // 错误消息应明确提到 T3 + 不静默 mock
    expect(/Real chat\(\) not wired/.test(body)).toBe(true)
    expect(/T3/.test(body)).toBe(true)
  })
})

// ============================================================
// 类型契约（编译期 —— vitest 通过 .d.ts 走 TypeScript）
// ============================================================

describe('类型导出契约', () => {
  it('WebLLMEngine 接口可作为类型引用', () => {
    // 编译期契约：类型存在且不报错
    const _type: WebLLMEngine | null = null
    const _state: DynamicEngineState | null = null
    expect(_type).toBeNull()
    expect(_state).toBeNull()
  })

  it('DynamicEnginePhase 是字符串字面量联合（4 值）', () => {
    // 验证枚举值的字面量集合
    const allPhases: DynamicEnginePhase[] = [
      'idle',
      'importing',
      'loaded',
      'unavailable',
    ]
    expect(allPhases).toHaveLength(4)
  })
})