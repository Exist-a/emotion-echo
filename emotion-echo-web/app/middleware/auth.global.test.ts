/**
 * P2-R2-4: 路由白名单单元测试（auth.global.ts）
 *
 * 验证白名单边界——之前 startsWith("/login") 被 "/loginxxx" "/Login" 绕过
 */
import { describe, it, expect } from 'vitest'

// 提取白名单匹配逻辑（与 middleware/auth.global.ts 保持同步）
const whiteListExact = new Set(['/login'])
const whiteListPrefix = ['/login/forget']

function isInWhiteList(path: string): boolean {
  return (
    whiteListExact.has(path) || whiteListPrefix.some((p) => path === p || path.startsWith(p + '/'))
  )
}

describe('P2-R2-4 auth whitelist prefix bypass', () => {
  it('精确匹配 /login', () => {
    expect(isInWhiteList('/login')).toBe(true)
  })

  it('拒绝 /loginxxx（绕过尝试）', () => {
    expect(isInWhiteList('/loginxxx')).toBe(false)
  })

  it('拒绝 /Login（大小写绕过尝试）', () => {
    expect(isInWhiteList('/Login')).toBe(false)
  })

  it('拒绝 /LOGIN', () => {
    expect(isInWhiteList('/LOGIN')).toBe(false)
  })

  it('拒绝 /loginbackdoor', () => {
    expect(isInWhiteList('/loginbackdoor')).toBe(false)
  })

  it('匹配 /login/forget 精确路径', () => {
    expect(isInWhiteList('/login/forget')).toBe(true)
  })

  it('匹配 /login/forget/verify（子路径）', () => {
    expect(isInWhiteList('/login/forget/verify')).toBe(true)
  })

  it('匹配 /login/forget/modify', () => {
    expect(isInWhiteList('/login/forget/modify')).toBe(true)
  })

  it('匹配 /login/forget/success', () => {
    expect(isInWhiteList('/login/forget/success')).toBe(true)
  })

  it('拒绝 /login/forgetx（前缀绕过尝试）', () => {
    expect(isInWhiteList('/login/forgetx')).toBe(false)
  })

  it('拒绝 /login/anything-else', () => {
    expect(isInWhiteList('/login/anything-else')).toBe(false)
  })

  it('受保护路由不被匹配', () => {
    expect(isInWhiteList('/chat/conversation')).toBe(false)
    expect(isInWhiteList('/settings')).toBe(false)
    expect(isInWhiteList('/')).toBe(false)
  })
})
