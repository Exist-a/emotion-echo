/**
 * webllmEngine 接口 + Stub 架构契约测试（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：纯静态源扫描 —— 验证：
 *   1. `interface WebLLMEngine` 必须声明 4 个方法：init / chat / abort / dispose
 *   2. `createStubEngine` 必须存在并返回 WebLLMEngine
 *   3. 文件**不能**静态引用 `@mlc-ai/web-llm`（production bundle 隔离，协议 §二 白名单）
 *   4. Stub 不能在 chat() 内调用 fetch / 直接触发网络（仅返回契约应答）
 *
 * 这是 .architecture.test.ts 静态源扫描形态 —— happy-dom mock 失真绕道
 * （v0.3 §F.2 #2 + memory `tdd-contract-vs-behavioral-tests.md`）。
 */
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = resolve(__filename, '..')
const ENGINE_SRC = readFileSync(
  resolve(__dirname, '..', 'webllmEngine.ts'),
  'utf8'
)

describe('webllmEngine.ts 静态契约', () => {
  it('必须声明 interface WebLLMEngine', () => {
    expect(/interface\s+WebLLMEngine\b/.test(ENGINE_SRC)).toBe(true)
  })

  it('WebLLMEngine 必须包含 init 方法', () => {
    expect(/\binit\s*\(/.test(ENGINE_SRC)).toBe(true)
  })

  it('WebLLMEngine 必须包含 chat 方法', () => {
    expect(/\bchat\s*\(/.test(ENGINE_SRC)).toBe(true)
  })

  it('WebLLMEngine 必须包含 abort 方法', () => {
    expect(/\babort\s*\(/.test(ENGINE_SRC)).toBe(true)
  })

  it('WebLLMEngine 必须包含 dispose 方法', () => {
    expect(/\bdispose\s*\(/.test(ENGINE_SRC)).toBe(true)
  })

  it('必须导出 createStubEngine 工厂函数', () => {
    expect(/export\s+(function|const)\s+createStubEngine\b/.test(ENGINE_SRC)).toBe(
      true
    )
  })

  it('不能静态 import @mlc-ai/web-llm（production bundle 隔离，协议 §二 白名单要求 dynamic import）', () => {
    // 匹配: import ... from '@mlc-ai/web-llm'
    expect(/from\s+['"]@mlc-ai\/web-llm['"]/.test(ENGINE_SRC)).toBe(false)
    // 匹配: import('@mlc-ai/web-llm') —— dynamic import 是允许的
    // 这里只断言"无 static import"
  })

  it('Stub 不能直接调用 fetch / XMLHttpRequest（真引擎由 T3 接入）', () => {
    // 提取 createStubEngine 函数体
    const fnMatch = ENGINE_SRC.match(
      /export\s+function\s+createStubEngine\s*\([\s\S]*?\n\}/
    )
    expect(fnMatch).not.toBeNull()
    const body = fnMatch?.[0] ?? ''
    expect(/\bfetch\s*\(/.test(body)).toBe(false)
    expect(/XMLHttpRequest/.test(body)).toBe(false)
  })

  it('chat() 必须返回 Promise<string> 或 AsyncIterable<string>（流式契约二选一）', () => {
    // 流式契约：要么 AsyncIterable（流），要么 Promise（一次性）
    // 简化：要求 chat 返回值声明包含 Promise 或 AsyncIterable
    const chatMatch = ENGINE_SRC.match(/chat\s*\([^)]*\)\s*:\s*([^\n;]+)/)
    expect(chatMatch).not.toBeNull()
    const retType = chatMatch?.[1] ?? ''
    expect(/Promise|AsyncIterable/.test(retType)).toBe(true)
  })
})