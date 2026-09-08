// app/composables/useFileUpload.test.ts
//
// Stage 58 PR-UP-2: 通用上传路径对齐测试（A2 文件上传前端契约）
//
// 目的：断言 useFileUpload 的上传端点与 BFF 真实现的 /api/v1/uploads/:kind 对齐
//
// 调研依据：
//   - emotion-echo-web-bff/internal/handler/upload_handler.go（PR-UP-1 真实现）
//   - emotion-echo-web/app/lib/apiRoutes.ts（PR-UP-2 转正主路径 /uploads/*）
//   - emotion-echo-web/app/composables/useApi.ts getBaseUrl() 已含 /api/v1 后缀
//     （request 函数做 `${getBaseUrl()}${url}` 拼接，前端路径不应再带 /api/v1）

import { describe, it, expect } from 'vitest'
import { API_ROUTES } from '../lib/apiRoutes'

describe('useFileUpload API routes alignment (Stage 58 PR-UP-2)', () => {
  it('主路径 /uploads/image 已注册且不复数', () => {
    expect(API_ROUTES.uploadImage.path).toBe('/uploads/image')
    expect(API_ROUTES.uploadImage.method).toBe('POST')
  })

  it('主路径 /uploads/video 已注册', () => {
    expect(API_ROUTES.uploadVideo.path).toBe('/uploads/video')
    expect(API_ROUTES.uploadVideo.method).toBe('POST')
  })

  it('主路径 /uploads/file 已注册', () => {
    expect(API_ROUTES.uploadFile.path).toBe('/uploads/file')
    expect(API_ROUTES.uploadFile.method).toBe('POST')
  })

  it('knownOrphans 已不含 uploadImage/Video/File 孤儿路径（PR-UP-2 转正后）', () => {
    const orphanPaths = Object.values(API_ROUTES.knownOrphans).map((r) => r.path)
    expect(orphanPaths).not.toContain('/upload/image')
    expect(orphanPaths).not.toContain('/upload/video')
    expect(orphanPaths).not.toContain('/upload/file')
    // 只剩 faceEmotionOrphan 死代码
    expect(orphanPaths).toContain('/face/emotion')
  })

  it('主路径与 BFF upload_handler.go 注册路径对齐（/uploads/:kind）', () => {
    // BFF 路由：r.POST("/api/v1/uploads/:kind", h.upload)
    // 前端路径：/uploads/:kind（不含 /api/v1 前缀，由 useApi.ts getBaseUrl() 拼接）
    const bffPrefix = '/api/v1'
    const fullPaths = [
      `${bffPrefix}${API_ROUTES.uploadImage.path}`,
      `${bffPrefix}${API_ROUTES.uploadVideo.path}`,
      `${bffPrefix}${API_ROUTES.uploadFile.path}`,
    ]
    // 断言前端路径 + /api/v1 前缀 === BFF handler 注册路径（复数）
    expect(fullPaths).toEqual([
      '/api/v1/uploads/image',
      '/api/v1/uploads/video',
      '/api/v1/uploads/file',
    ])
  })
})