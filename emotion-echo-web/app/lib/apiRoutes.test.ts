// app/lib/apiRoutes.test.ts
//
// Sprint 1 PR-2 (2026-09-04): 前端 API 路径契约测试
//
// 目的：扫 Emotion-Echo-Web/app 下所有 .vue / .ts 文件，提取硬编码的 API 路径字面量，
//       断言每条都在 API_ROUTES 注册过（含 knownOrphans）。
//       防止以下漂移：
//         - 后端改路径，前端忘了同步（→ 404 / 业务失败）
//         - 前端加了新端点调，但 API_ROUTES 没收（→ 契约盲点）
//         - 有人随手改 typo（/auth/login → /auth/Login）
//
// 设计要点（调研确认）：
//   - vitest include glob: app/**/*.{test,spec}.ts —— 不包含 .vue，所以契约测试放 .ts
//   - fast-glob 在 devDependencies 已装（v3.3.3）
//   - API_ROUTES 定义在 ./apiRoutes.ts（含 26 条标准路径 + 5 条 knownOrphans）
//   - 5 条 knownOrphans：PR-4 落地前的孤儿（/face/emotion, /upload/image, /upload/video,
//     /upload/file, /voice/upload）——这些已被前端调用但 BFF 未实现；契约测试不 fail
//
// 调研依据：
//   - emotion-echo-web-bff/main.go 路由清单（PR-1 测试已锁 27 条 + EmotionQ 3 条）
//   - Emotion-Echo-Web/app/composables/{useFileUpload,useFaceEmotion,useVoiceRecorder}.ts
//   - Emotion-Echo-Web/app/stores/{user,conversation,message}.ts
//   - todo-pile-2026-09-04.md C8 + Sprint 1 plan §PR-2

import { describe, it, expect } from 'vitest'
import fg from 'fast-glob'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { API_ROUTES } from './apiRoutes'

const ROOT = path.resolve(__dirname, '..', '..')
const APP_DIR = path.join(ROOT, 'app')

// 从单引号 / 双引号 / 反引号字符串字面量里提取形如 /api/v1/... 的路径
const PATH_REGEX = /['"`](?:\/api\/v1)?(\/[a-z][a-z0-9_\-/{}:]+)['"`]/gi

// 排除文件：apiRoutes 自身 + 本测试文件 + 文档字符串
function shouldSkip(filePath: string): boolean {
  const rel = path.relative(ROOT, filePath).replace(/\\/g, '/')
  if (rel.includes('app/lib/apiRoutes')) return true
  if (rel.endsWith('.test.ts') || rel.endsWith('.spec.ts')) return true
  return false
}

describe('API routes contract', () => {
  it('every hardcoded API path in app/**/*.{vue,ts} is registered in API_ROUTES', async () => {
    const files = await fg(['app/**/*.{vue,ts}'], {
      cwd: ROOT,
      absolute: true,
      ignore: ['**/node_modules/**'],
    })

    const hardcoded = new Set<string>()
    const perFileHits: Record<string, string[]> = {}

    for (const f of files) {
      if (shouldSkip(f)) continue
      const src = readFileSync(f, 'utf8')
      const hits: string[] = []
      let m: RegExpExecArray | null
      // 重置 lastIndex（regex 带 g flag 时跨调用会保留 state）
      PATH_REGEX.lastIndex = 0
      while ((m = PATH_REGEX.exec(src)) !== null) {
        const p = m[1]
        // 只关心"业务 API 路径"——以业务前缀起
        if (
          p.startsWith('/auth') ||
          p.startsWith('/user') ||
          p.startsWith('/users') ||
          p.startsWith('/conversations') ||
          p.startsWith('/messages') ||
          p.startsWith('/reports') ||
          p.startsWith('/surveys') ||
          p.startsWith('/upload') ||
          p.startsWith('/uploads') ||
          p.startsWith('/voice') ||
          p.startsWith('/face') ||
          p.startsWith('/tts') ||
          p.startsWith('/ai/') ||
          p.startsWith('/multimodal') ||
          p.startsWith('/user-behavior') ||
          p.startsWith('/mental-health')
        ) {
          hardcoded.add(p)
          hits.push(p)
        }
      }
      if (hits.length > 0) {
        perFileHits[path.relative(ROOT, f).replace(/\\/g, '/')] = hits
      }
    }

    // 列出每个文件提取出的路径（便于人工 review + 调试）
    if (Object.keys(perFileHits).length > 0) {
      const sortedFiles = Object.keys(perFileHits).sort()
      // 仅打印前 30 行避免淹没测试输出
      const lines = sortedFiles.slice(0, 30).map(
        (f) => `  ${f}: ${perFileHits[f].join(', ')}`,
      )
      if (sortedFiles.length > 30) lines.push(`  ... (${sortedFiles.length - 30} more)`)
      console.log('Scanned paths by file:\n' + lines.join('\n'))
    }

    // 断言：硬编码路径集合 ⊆ API_ROUTES 已知集合（递归展开 knownOrphans 等嵌套段）
    function flatten(obj: any): ApiRoute[] {
      const out: ApiRoute[] = []
      for (const v of Object.values(obj)) {
        if (v && typeof v === 'object' && 'method' in v && 'path' in v) {
          out.push(v as ApiRoute)
        } else if (v && typeof v === 'object') {
          out.push(...flatten(v))
        }
      }
      return out
    }
    const known = new Set(flatten(API_ROUTES).map((r) => r.path))
    const orphans: string[] = []
    for (const p of hardcoded) {
      if (!known.has(p)) orphans.push(p)
    }

    expect(
      orphans,
      `发现 ${orphans.length} 条硬编码路径不在 API_ROUTES 中:\n  ${orphans.join('\n  ')}\n` +
        `请新增到 app/lib/apiRoutes.ts（标准路径）或加 knownOrphans（短期未实现）`,
    ).toEqual([])
  }, 30_000)

  it('API_ROUTES 至少包含 26 条标准路径 + 5 条 knownOrphans', () => {
    // 展开嵌套 knownOrphans 段（递归处理任意深度嵌套）
    function flatten(obj: any): ApiRoute[] {
      const out: ApiRoute[] = []
      for (const v of Object.values(obj)) {
        if (v && typeof v === 'object' && 'method' in v && 'path' in v) {
          out.push(v as ApiRoute)
        } else if (v && typeof v === 'object') {
          out.push(...flatten(v))
        }
      }
      return out
    }
    const all = flatten(API_ROUTES)
    expect(all.length).toBeGreaterThanOrEqual(31)
  })

  it('API_ROUTES 不含空 (method,path) 组合或重复组合', () => {
    function flatten(obj: any): ApiRoute[] {
      const out: ApiRoute[] = []
      for (const v of Object.values(obj)) {
        if (v && typeof v === 'object' && 'method' in v && 'path' in v) {
          out.push(v as ApiRoute)
        } else if (v && typeof v === 'object') {
          out.push(...flatten(v))
        }
      }
      return out
    }
    // RESTful 设计允许同一 path 不同 method 共享（如 GET/POST /conversations），
    // 所以重复判断是 (method, path) 二元组都等才算重
    const routes = flatten(API_ROUTES)
    const seen = new Set<string>()
    const dups: string[] = []
    for (const r of routes) {
      if (!r.method || !r.path) {
        throw new Error(`API_ROUTES 出现空 method/path: ${JSON.stringify(r)}`)
      }
      const key = `${r.method} ${r.path}`
      if (seen.has(key)) dups.push(key)
      seen.add(key)
    }
    expect(dups, `API_ROUTES 重复 (method, path) 组合: ${dups.join(', ')}`).toEqual([])
  })
})
