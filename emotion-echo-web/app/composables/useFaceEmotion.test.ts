import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

/**
 * Stage 97 PR-7 · P0-R2-2 字面量契约测试
 *
 * 背景：原 useFaceEmotion 调 orphan path `/face/emotion`，BFF 未实现，
 * 永远 404 但 UI 假装工作（情绪未刷新也"检测中"）。
 *
 * 修复：改调 `/multimodal/analyze`（multipart/form-data，kind=image），
 * 后端真有 handler（Stage 23 实现）。
 *
 * 本测试钉死 5 个字面量契约：
 *   1. 源码不出现 orphan 路径 `/face/emotion`
 *   2. 源码不出现 orphan apiRoutes key `faceEmotionOrphan`
 *   3. 源码出现 `/multimodal/analyze`（multimodalAnalyze apiRoute）
 *   4. 源码构建 FormData，含 `kind='image'` + `persist='false'`
 *   5. 源码用 multipart/form-data（FormData 对象）而非 JSON
 *
 * 字面量断言 vs 行为测试：本 composable 依赖 MediaStream + canvas + video
 * element 等 happy-dom 不实现的浏览器 API，行为测试成本过高且不稳；
 * 字面量断言在测试金字塔里更低成本但能锁死"修复方向正确"。
 */

describe('useFaceEmotion · P0-R2-2 字面量契约', () => {
  const src = readFileSync(
    resolve(__dirname, './useFaceEmotion.ts'),
    'utf8'
  )

  it('不含 orphan 路径 /face/emotion', () => {
    expect(src).not.toContain('/face/emotion')
  })

  it('不含 orphan apiRoutes key faceEmotionOrphan', () => {
    expect(src).not.toContain('faceEmotionOrphan')
  })

  it('必须调 /multimodal/analyze (API_ROUTES.multimodalAnalyze)', () => {
    expect(src).toContain('multimodalAnalyze')
    expect(src).toContain('/multimodal/analyze')
  })

  it('构建 multipart/form-data (FormData) 而非 JSON', () => {
    expect(src).toContain('new FormData()')
    expect(src).toContain("formData.append('kind', 'image')")
    expect(src).toContain("formData.append('persist', 'false')")
  })

  it('从 base64 还原为 Blob 上传, 而不是 JSON base64 字符串', () => {
    // 原实现: post({ imageBase64: ... })  → 调 orphan /face/emotion
    // 新实现: formData.append('file', new Blob([...]))  → 走 /multimodal/analyze
    expect(src).toContain('new Blob([')
    expect(src).toContain("formData.append('file'")
  })
})