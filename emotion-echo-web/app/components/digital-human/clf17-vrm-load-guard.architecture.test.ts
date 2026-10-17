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
// 真实成因链（亲核，file:line 见下）：
//   ① `loadVRM()` 由 `onMounted` 触发，`await loader.loadAsync(props.modelPath)` 要下载
//      **26.7MB** 的 VRM ⇒ 这个 await 有**秒级窗口**；
//   ② 期间若组件被卸载，`onUnmounted` → `disposeThreeJS()` 把模块级 `scene` **置为 null**；
//   ③ await 恢复后代码**不再看 scene 一眼**，直接 `scene.add(...)` ⇒ 空指针抛错。
//   入口处的早退检查发生在 await **之前**，挡不住这条路径。
//
// ⚠️ 修订记录（2026-10-16，**第二方核对抓出的假绿**）：
//   本钉首版直接对**原文**做 `indexOf('scene.add(')` —— 结果命中的是守卫上方
//   **注释里**的 `` `scene.add(...)` `` 字样，断言窗口只覆盖到注释；而注释里恰好也写着
//   `` `if (!scene)` `` ⇒ **删掉真实守卫后本钉仍然全绿**（已被独立复现）。
//   现改为：**先剥离注释**，再在纯代码里定位「await → 首个 scene.add」，并要求窗口内
//   出现"**带 return 的** scene 空值/已销毁守卫"。去掉守卫会让本钉**如期变红**（已负向验证）。
//
// 说明：组件强依赖 WebGL，happy-dom 下无法做**渲染级**行为断言（见项目既有体例
// `message-status-single-source.architecture.test.ts`）；本钉是**代码级结构不变量**，
// **不能**替代浏览器实测 —— 运行时验收另见
// docs/closure-roadmap/stages/cl-09-backlog-paydown/evidence/clf17-iab-*.md。

const __dirname = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(resolve(__dirname, 'DigitalHuman.client.vue'), 'utf8')

/**
 * 剥离 `//` 行注释与 `/* *\/` 块注释（保留换行以维持行号近似）。
 * 够用即可：本文件的守卫区域没有含 `//` 的字符串/正则字面量。
 */
function stripComments(src: string): string {
  const noBlock = src.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
  return noBlock
    .split('\n')
    .map((line) => {
      const i = line.indexOf('//')
      // 只当 `//` 不在引号里时才算注释（本文件足够）
      if (i >= 0 && (line.slice(0, i).match(/['"`]/g) || []).length % 2 === 0) {
        return line.slice(0, i)
      }
      return line
    })
    .join('\n')
}

const CODE = stripComments(SRC)

/** 取 loadVRM 函数体（到 disposeThreeJS 定义为止） */
function loadVrmCode(src: string): string {
  const start = src.indexOf('const loadVRM = async () => {')
  expect(start, '找不到 loadVRM 定义（改名了？请同步本钉）').toBeGreaterThan(-1)
  const end = src.indexOf('const disposeThreeJS', start)
  expect(end, '找不到 loadVRM 的结束边界').toBeGreaterThan(start)
  return src.slice(start, end)
}

describe('DigitalHuman loadVRM 空场景守卫契约（CL-F-17）', () => {
  it('扫到了源码，且剥离注释后仍非空（防剥离失败导致假绿）', () => {
    expect(SRC.length).toBeGreaterThan(500)
    expect(SRC).toContain('DigitalHuman')
    expect(CODE.length, '剥离注释后不应为空').toBeGreaterThan(200)
    // 防"剥离把代码也剥没了"：剥离后的代码里必须仍有真实调用
    expect(CODE).toContain('await loader.loadAsync')
    expect(CODE).toContain('scene.add(')
  })

  it('await loadAsync 之后、首个**代码级** scene.add 之前，必须有带 return 的守卫', () => {
    const body = loadVrmCode(CODE)

    const awaitIdx = body.indexOf('await loader.loadAsync')
    expect(awaitIdx, 'loadVRM 应调用 await loader.loadAsync').toBeGreaterThan(-1)

    const addIdx = body.indexOf('scene.add(', awaitIdx)
    expect(
      addIdx,
      'await 之后应有**代码级**的 scene.add（若只找到注释里的，说明剥离失败或结构变了）',
    ).toBeGreaterThan(awaitIdx)

    const between = body.slice(awaitIdx, addIdx)
    // 守卫形态：`if (disposed || !scene) { return }` / `if (!scene) return` / `if (scene === null) return`
    const guardWithReturn =
      /if\s*\([^)]*!\s*scene[^)]*\)\s*\{[^}]*\breturn\b[^}]*\}/.test(between) ||
      /if\s*\([^)]*!\s*scene[^)]*\)\s*return\b/.test(between) ||
      /if\s*\([^)]*scene\s*===\s*null[^)]*\)\s*\{?[^}]*\breturn\b/.test(between)
    expect(
      guardWithReturn,
      'await 恢复后必须有「判 scene 为空即 return」的守卫 —— 否则组件在下载模型期间被卸载时抛 TypeError（CL-F-17）',
    ).toBe(true)
  })

  it('组件卸载时置位"已销毁"标记，供 await 恢复后判定', () => {
    const unmountIdx = SRC.indexOf('onUnmounted(')
    expect(unmountIdx).toBeGreaterThan(-1)
    const unmountBody = SRC.slice(unmountIdx, unmountIdx + 400)
    expect(
      /disposed\s*=\s*true/.test(unmountBody),
      'onUnmounted 回调体内应置位 disposed（供 await 后判定，且必须在 dispose 之前）',
    ).toBe(true)
    // 顺序：先置位，再拆场景（反了就有"已置 null、标记还没置"的窗口）
    const setIdx = unmountBody.search(/disposed\s*=\s*true/)
    const disposeIdx = unmountBody.indexOf('disposeThreeJS(')
    expect(disposeIdx).toBeGreaterThan(-1)
    expect(setIdx, 'disposed 必须**先于** disposeThreeJS() 置位').toBeLessThan(disposeIdx)
  })
})
