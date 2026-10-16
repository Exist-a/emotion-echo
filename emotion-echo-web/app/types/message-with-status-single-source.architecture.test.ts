import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'

// CL-F-18 契约钉：`MessageWithStatus` 的**声明点**必须收敛且各有明确边界。
//
// 现场（2026-10-14 D 轨核出，账本 CL-F-18）全仓 **3 处**同名声明、形态各不相同：
//   ① types/api.ts                          —— extends MessageItem   { status?: }              （API DTO）
//   ② types/conversation/conversationMessagesType.ts —— extends StoredMessage { status: ; retryCount: }（存储行）
//   ③ utils/messageCache.ts（文件内 private） —— extends StoredMessage { status?: ; retryCount?: }（与 ② 基类相同）
//
// 本钉的收敛动作（2026-10-16，CL 遗留清销第 1 批）：
//   · **删除 ③** —— 它逐字等于已导出的 `StoredMessage`（`status?`/`retryCount?` 本就可选），是纯冗余副本；
//   · 保留 ①②，并**各自钉住边界注释**（两者基类不同且 `sender` 取值大小写不同：
//     api 侧 `'user' | 'ai'`、存储侧 `'user' | 'AI'`）—— 这一处**结构性不等价**的收敛
//     被用户裁定 `CL-D-12`（"只登记不动"）阻塞，故此处**只钉边界、不改语义**。
//
// 回退实现（再在别处声明一份）会让本钉如期变红。

const __dirname = dirname(fileURLToPath(import.meta.url))
const APP_ROOT = resolve(__dirname, '..') // <web>/app

/** 允许声明 `MessageWithStatus` 的两个文件（相对 app/） */
const ALLOWED = ['types/api.ts', 'types/conversation/conversationMessagesType.ts']

/** 曾经的第三份冗余副本 —— 必须消失 */
const FORMER_DUPLICATE = 'utils/messageCache.ts'

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const abs = join(dir, entry)
    if (statSync(abs).isDirectory()) {
      walk(abs, out)
      continue
    }
    if (/\.(?:ts|vue)$/.test(entry)) out.push(abs)
  }
  return out
}

const PROD = walk(APP_ROOT)
  .filter((abs) => !/\.(?:test|spec)\.ts$/.test(abs))
  .map((abs) => ({
    rel: abs.slice(APP_ROOT.length + 1).replace(/\\/g, '/'),
    src: readFileSync(abs, 'utf8'),
  }))

/** 声明形态：`export interface MessageWithStatus` / `interface MessageWithStatus` / `type MessageWithStatus =` */
const DECL_RE = /^\s*(?:export\s+)?(?:interface|type)\s+MessageWithStatus\b/m

describe('MessageWithStatus 声明点收敛契约（CL-F-18）', () => {
  it('扫到了源码（防扫空导致假绿）', () => {
    expect(PROD.length, 'PROD 为空说明目录遍历失效').toBeGreaterThan(50)
  })

  it('声明点恰好收敛为 2 处，且都在允许清单内', () => {
    const decls = PROD.filter((f) => DECL_RE.test(f.src))
      .map((f) => f.rel)
      .sort()
    expect(decls, `声明点与允许清单不一致：${decls.join(', ')}`).toEqual([...ALLOWED].sort())
  })

  it('曾经的第三份冗余副本（utils/messageCache.ts）不得再声明 MessageWithStatus', () => {
    const f = PROD.find((x) => x.rel === FORMER_DUPLICATE)
    expect(f, `找不到文件 ${FORMER_DUPLICATE}（改名了？请同步本钉）`).toBeTruthy()
    expect(DECL_RE.test(f!.src), `${FORMER_DUPLICATE} 不应再声明 MessageWithStatus`).toBe(false)
  })

  it('utils/messageCache.ts 改为引用公共存储行类型 StoredMessage', () => {
    const f = PROD.find((x) => x.rel === FORMER_DUPLICATE)!
    expect(
      /\bStoredMessage\b/.test(f.src),
      '删掉私声明后应改用 StoredMessage（否则类型会漂成 any）',
    ).toBe(true)
  })

  it('两处保留的声明各自写明边界（为何不能合并）', () => {
    for (const rel of ALLOWED) {
      const f = PROD.find((x) => x.rel === rel)
      expect(f, `找不到文件 ${rel}`).toBeTruthy()
      const idx = f!.src.search(DECL_RE)
      // 声明点前 800 字符内应出现 CL-F-18 或"边界/不等价"说明
      const around = f!.src.slice(Math.max(0, idx - 800), idx + 200)
      expect(
        /CL-F-18|边界|不等价|API DTO|存储行/.test(around),
        `${rel} 的 MessageWithStatus 应带边界注释（说明与另一处的差异与为何不合并）`,
      ).toBe(true)
    }
  })
})
