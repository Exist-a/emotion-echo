import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

// CL-F-17 契约钉：`loadVRM` 必须**在 await 之后重新校验场景**再碰 `scene.add`。
//
// 现场（2026-10-15 用户手动测试，账本 CL-F-17）：
//   控制台 `TypeError: Cannot read properties of null (reading 'add')`（loadVRM 内 scene.add）
//   + `[DigitalHuman] Scene is not initialized!`，3D 形象不显示。
//
// 真实成因链（亲核代码，2026-10-16）：
//   ① `loadVRM()` 由 `onMounted` 触发，`await loader.loadAsync(props.modelPath)` 要下载
//      **26.7MB** 的 VRM 模型 ⇒ 这个 await 有**秒级窗口**；
//   ② 期间若组件被卸载（路由切走 / definePageMeta key 变化 / HMR），`onUnmounted` →
//      `disposeThreeJS()` 把模块级 `scene` **置为 null**（`:444`）；
//   ③ await 恢复后代码**不再看 scene 一眼**，直接 `scene.add(...)` ⇒ 空指针抛错。
//   入口处的早退检查（`if (!scene) return`）发生在 await **之前**，挡不住这条路径。
//
// 本钉锁的是**结构性不变量**（不是"某行字符串存在"）：
//   在 `await loader.loadAsync(...)` 之后、第一次 `scene.add` 之前，必须出现一次
//   `scene` 的空值/已销毁再校验。回退实现（删掉该守卫）会让本钉如期变红。
//
// 说明：本组件强依赖 WebGL，happy-dom 下无法做**渲染级**行为断言（参见项目既有体例：
// 依赖浏览器 GPU API 的组件用源码级契约钉，见 message-status-single-source.architecture.test.ts）。
// 运行时复现路径记录在 docs/plans/cl-legacy-paydown-2026-10-16.md。

const __dirname = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(resolve(__dirname, 'DigitalHuman.client.vue'), 'utf8')

/** 取 loadVRM 函数体（从 `const loadVRM = async () => {` 到下一个顶层 `const ` / `}` 边界） */
function loadVrmBody(src: string): string {
  const start = src.indexOf('const loadVRM = async () => {')
  expect(start, '找不到 loadVRM 定义（改名了？请同步本钉）').toBeGreaterThan(-1)
  // 到 retryLoadVRM 之前的 disposeThreeJS 定义为止
  const end = src.indexOf('const disposeThreeJS', start)
  expect(end, '找不到 loadVRM 的结束边界').toBeGreaterThan(start)
  return src.slice(start, end)
}

describe('DigitalHuman loadVRM 空场景守卫契约（CL-F-17）', () => {
  it('扫到了源码（防文件路径漂移导致假绿）', () => {
    expect(SRC.length).toBeGreaterThan(500)
    expect(SRC).toContain('DigitalHuman')
  })

  it('await loadAsync 之后、首次 scene.add 之前，必须重新校验 scene', () => {
    const body = loadVrmBody(SRC)

    const awaitIdx = body.indexOf('await loader.loadAsync')
    expect(awaitIdx, 'loadVRM 应调用 await loader.loadAsync').toBeGreaterThan(-1)

    const addIdx = body.indexOf('scene.add(', awaitIdx)
    expect(addIdx, 'await 之后应把模型加进场景（找不到 scene.add 说明结构变了）').toBeGreaterThan(
      awaitIdx,
    )

    const between = body.slice(awaitIdx, addIdx)
    // 守卫形态：`if (!scene ...)` / `if (disposed ...)` / `if (scene === null)`
    const hasGuard =
      /if\s*\(\s*!scene\b/.test(between) ||
      /if\s*\(\s*scene\s*===\s*null\s*\)/.test(between) ||
      /if\s*\(\s*disposed\b/.test(between)
    expect(
      hasGuard,
      'await 恢复后必须先判 `!scene`（或已销毁）再 scene.add —— 否则组件在下载模型期间被卸载时抛 TypeError（CL-F-17）',
    ).toBe(true)
  })

  it('组件卸载时置位"已销毁"标记，供 await 恢复后判定', () => {
    expect(
      /disposed\s*=\s*true/.test(SRC) || /disposed\.value\s*=\s*true/.test(SRC),
      'onUnmounted 应把 disposed 置 true，否则 await 恢复后无从知道组件已卸载',
    ).toBe(true)
    // 卸载钩子里必须真的置位
    const unmountIdx = SRC.indexOf('onUnmounted(')
    expect(unmountIdx).toBeGreaterThan(-1)
    const unmountBody = SRC.slice(unmountIdx, unmountIdx + 400)
    expect(
      /disposed\s*=\s*true/.test(unmountBody) || /disposed\.value\s*=\s*true/.test(unmountBody),
      'onUnmounted 回调体内应置位 disposed（当前只在别处置位或未置位）',
    ).toBe(true)
  })
})
