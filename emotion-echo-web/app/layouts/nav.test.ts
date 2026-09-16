import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const NAV_SRC = readFileSync(resolve(__dirname, 'nav.vue'), 'utf8')

// nav.vue 高度链路公共合同 (Stage 104):
//   - 桌面 (.app-content, .page-content) 使用 height: 100vh/100dvh, 而不是 min-height
//     (避免内容长于视口时高度无限拉伸, 导致 chat-main 无法撑满剩余空间)
//   - 桌面 .chat-area (来自 conversation/index.vue) 的父级 .conversation-page 必须有界高度
//   - 移动端断点 (@media max-width:768px) 同样用 height 而非 min-height
//   - 数字人 wrapper (.digital-human-wrapper) 必须定位 fixed, 不参与 .chat-main 的 overflow
describe('nav.vue height-chain contract (Stage 104)', () => {
  const firstBlock = (selector: string) => {
    // 取第一个匹配 .selector { ... } 的块,允许内部含 ;
    const re = new RegExp(`\\${selector}\\s*\\{([\\s\\S]*?)\\}`)
    return NAV_SRC.match(re)?.[0] ?? ''
  }

  it('desktop .app-content uses height (not min-height) so .chat-main can fill remaining space', () => {
    const block = firstBlock('.app-content')
    expect(block).toMatch(/\bheight\s*:/)
    expect(block).not.toMatch(/min-height/)
  })

  it('desktop .page-content uses height (no min-height except 0 for flex overflow)', () => {
    const block = firstBlock('.page-content')
    expect(block).toMatch(/\bheight\s*:/)
    // min-height: 0 是 flex 容器允许子元素正确 overflow 的合法用法
    const strayMinHeight = (block.match(/min-height\s*:\s*([^;]+)/g) || [])
      .filter((m) => !/:\s*0(?:\s*!important)?\s*$/.test(m))
    expect(strayMinHeight, `多余的 min-height: ${strayMinHeight.join(';')}`).toEqual([])
  })

  it('mobile breakpoint (.app-content) uses height, not min-height', () => {
    // 取 @media (max-width: 768px) {...} 完整块,搜 .app-content 子块
    const mobileMedia = NAV_SRC.match(/@media\s*\(max-width:\s*768px\)\s*\{([\s\S]*?)\n\}/)?.[1] ?? ''
    const block = mobileMedia.match(/\.app-content\s*\{([\s\S]*?)\}/)?.[0] ?? ''
    expect(block).toMatch(/\bheight\s*:/)
    expect(block).not.toMatch(/min-height/)
  })

  it('uses dynamic viewport unit (100dvh) so mobile address bar does not break layout', () => {
    expect(NAV_SRC).toMatch(/100dvh/)
  })
})