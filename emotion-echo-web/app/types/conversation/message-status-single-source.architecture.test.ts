import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'

// CL-05 Phase 1 · T-13：MessageStatus「单一来源」契约。
//
// 现状（2026-10-14 亲核）：「单一来源」名不副实 ——
//   ① 全仓有 **2 处** `export type MessageStatus =`：stores/message.ts（5 值）
//      与 types/conversation/conversationMessagesType.ts（**4 值**，缺 truncated）
//      —— 两处取值集合就不一致；
//   ② types/api.ts 还有一处**内联字面量联合**（第三份副本）。
// 本钉用源码级契约把三件事锁死：定义点唯一、取值集合唯一、其余文件引用而非重声明。
//
// 依据：docs/closure-roadmap/stages/cl-05-message-pipeline/plan.md §2 T-13 / §3 Phase 1。
// 体例参照 app/stores/default-avatar-asset-exists.architecture.test.ts（源码扫描型架构钉）。

const __dirname = dirname(fileURLToPath(import.meta.url))
const APP_ROOT = resolve(__dirname, '../..') // <web>/app

/** 唯一合法定义点（相对 app/ 的路径） */
const CANONICAL = 'types/conversation/conversationMessagesType.ts'

/** 状态取值集合（超集；truncated 亦在列） */
const STATUS_VALUES = ['sending', 'sent', 'failed', 'streaming', 'truncated']

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const abs = join(dir, entry)
    if (statSync(abs).isDirectory()) {
      walk(abs, out)
      continue
    }
    if (/\.(ts|vue)$/.test(entry)) out.push(abs)
  }
  return out
}

interface SourceFile {
  rel: string
  src: string
}

const PROD: SourceFile[] = walk(APP_ROOT)
  // 测试文件自身不参与判定（否则本文件会被自己扫成"重复声明"）
  .filter((abs) => !/\.(test|spec)\.ts$/.test(abs))
  .map((abs) => ({
    rel: abs.slice(APP_ROOT.length + 1).replace(/\\/g, '/'),
    src: readFileSync(abs, 'utf8'),
  }))

const DECL_RE = /^\s*export\s+type\s+MessageStatus\s*=/m
/** 形如 `'sent' | 'failed'` 的**类型级**字面量联合（运行时字符串赋值不算） */
const UNION_RE = new RegExp(
  `'(?:${STATUS_VALUES.join('|')})'\\s*\\|\\s*'(?:${STATUS_VALUES.join('|')})'`,
)

describe('MessageStatus 单一来源契约（CL-05 Phase 1 · T-13）', () => {
  it('扫到了源码（防扫空导致假绿）', () => {
    expect(PROD.length, 'PROD 为空说明目录遍历失效').toBeGreaterThan(50)
    expect(
      PROD.some((f) => f.rel === CANONICAL),
      `canonical 文件未在扫描结果中：${CANONICAL}`,
    ).toBe(true)
  })

  it('全仓 `export type MessageStatus` 定义点唯一，且落在 types/conversation/conversationMessagesType.ts', () => {
    const defs = PROD.filter((f) => DECL_RE.test(f.src)).map((f) => f.rel)
    expect(defs, `重复/错误的定义点：${defs.join(', ')}`).toEqual([CANONICAL])
  })

  it('唯一来源的取值集合 = 5 值超集（含 truncated）', () => {
    const canonical = PROD.find((f) => f.rel === CANONICAL)
    expect(canonical, `缺少 canonical 文件 ${CANONICAL}`).toBeTruthy()
    const m = canonical!.src.match(/export\s+type\s+MessageStatus\s*=\s*([^\n]+)/)
    expect(m, 'canonical 文件应声明 MessageStatus 类型').toBeTruthy()
    const values = [...m![1].matchAll(/'([a-z]+)'/g)].map((x) => x[1]).sort()
    expect(values, '取值集合应与 5 值超集完全一致').toEqual([...STATUS_VALUES].sort())
  })

  it('其它文件不得内联重声明状态字面量联合（须引用单一来源）', () => {
    const offenders = PROD.filter((f) => f.rel !== CANONICAL && UNION_RE.test(f.src)).map(
      (f) => f.rel,
    )
    expect(offenders, `内联重复声明：${offenders.join(', ')}`).toEqual([])
  })
})
