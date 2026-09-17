import { describe, it, expect } from 'vitest'
import { readFileSync, existsSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname, join } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const USER_STORE_SRC = readFileSync(resolve(__dirname, 'user.ts'), 'utf8')

// Stage 112 修复 (Bug C)：/chat/user 页面头像显示破图，因为 user store 的
// 默认头像路径 /imgs/default-avatar.webp 在 dev mode 下 404。
// 本测试钉死契约：默认头像 fallback 字符串必须指向 dev server 上真实存在的资源。
describe('user store default avatar fallback contract (Stage 112 bug C)', () => {
  function extractFallbackPath(src: string): string | null {
    const m = src.match(/userInfo\.value\?\.avatar\s*\|\|\s*['"]([^'"]+)['"]/)
    return m?.[1] ?? null
  }

  it('user store 必须声明一个默认头像 fallback 路径', () => {
    const fallback = extractFallbackPath(USER_STORE_SRC)
    expect(fallback, 'user store 应有 avatar fallback 路径').toBeTypeOf('string')
    expect(fallback).toMatch(/^\/imgs\//)
  })

  it('fallback 路径对应的 public 资源必须存在且为非空 webp/jpg/png 文件', () => {
    const fallback = extractFallbackPath(USER_STORE_SRC)
    expect(fallback, 'user store 应有 avatar fallback 路径').toBeTypeOf('string')
    const publicRoot = resolve(__dirname, '../../public')
    const assetPath = join(publicRoot, fallback!.replace(/^\//, ''))
    expect(existsSync(assetPath), `${fallback} 文件不存在 → 头像破图`).toBe(true)
    const stat = statSync(assetPath)
    expect(stat.size, `${fallback} 不能是 0 字节占位`).toBeGreaterThan(0)
  })
})