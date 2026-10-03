import { describe, it, expect } from 'vitest'
import { resolveObjectUrl } from './objectUrl'

/**
 * E2E-27 M1 / F-116（ADR-2026-09 决策 1）前端侧契约：
 * 服务端下发的对象 URL 改为网关相对路径（/api/v1/...），渲染 <img> 前必须解析成
 * 网关绝对地址；三类值必须原样透传（legacy 绝对地址 / 本地 blob / 静态资源）。
 * 与 ChatFile.getFullUrl / getFullAudioUrl 同语义，抽成共享 util。
 */
describe('resolveObjectUrl', () => {
  const apiBase = 'http://localhost:19080/api/v1'

  it('网关相对路径解析为网关绝对地址（去 /api/v1 后拼接，避免 /api/v1 重复）', () => {
    expect(resolveObjectUrl('/api/v1/user/avatar/image/7-2cdae8ed.jpg', apiBase)).toBe(
      'http://localhost:19080/api/v1/user/avatar/image/7-2cdae8ed.jpg',
    )
  })

  it('legacy 绝对地址原样透传（存量 DB 数据不回填，惰性兼容）', () => {
    const legacy = 'http://localhost:9000/avatars/avatars/1-2ec01835.png'
    expect(resolveObjectUrl(legacy, apiBase)).toBe(legacy)
  })

  it('静态资源路径原样透传（默认头像 /imgs/...）', () => {
    expect(resolveObjectUrl('/imgs/default-avatar.webp', apiBase)).toBe('/imgs/default-avatar.webp')
  })

  it('本地 blob URL 原样透传（上传前端预览）', () => {
    const blob = 'blob:http://localhost:3000/abc-123'
    expect(resolveObjectUrl(blob, apiBase)).toBe(blob)
  })

  it('空值返回空串', () => {
    expect(resolveObjectUrl('', apiBase)).toBe('')
    expect(resolveObjectUrl(null, apiBase)).toBe('')
    expect(resolveObjectUrl(undefined, apiBase)).toBe('')
  })
})
