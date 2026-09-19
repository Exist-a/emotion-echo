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

  it('desktop .app-content uses height (no min-height except 0 for flex overflow)', () => {
    const block = firstBlock('.app-content')
    expect(block).toMatch(/\bheight\s*:/)
    // min-height: 0 是 flex 容器允许子元素正确 overflow 的合法用法
    const strayMinHeight = (block.match(/min-height\s*:\s*([^;]+)/g) || []).filter(
      (m) => !/:\s*0(?:\s*!important)?\s*$/.test(m),
    )
    expect(strayMinHeight, `多余的 min-height: ${strayMinHeight.join(';')}`).toEqual([])
  })

  it('desktop .page-content uses height (no min-height except 0 for flex overflow)', () => {
    const block = firstBlock('.page-content')
    expect(block).toMatch(/\bheight\s*:/)
    // min-height: 0 是 flex 容器允许子元素正确 overflow 的合法用法
    const strayMinHeight = (block.match(/min-height\s*:\s*([^;]+)/g) || []).filter(
      (m) => !/:\s*0(?:\s*!important)?\s*$/.test(m),
    )
    expect(strayMinHeight, `多余的 min-height: ${strayMinHeight.join(';')}`).toEqual([])
  })

  it('mobile breakpoint (.app-content) uses height, not min-height (min-height:0 excepted)', () => {
    const mobileMedia =
      NAV_SRC.match(/@media\s*\(max-width:\s*768px\)\s*\{([\s\S]*?)\n\}/)?.[1] ?? ''
    const block = mobileMedia.match(/\.app-content\s*\{([\s\S]*?)\}/)?.[0] ?? ''
    expect(block).toMatch(/\bheight\s*:/)
    const strayMinHeight = (block.match(/min-height\s*:\s*([^;]+)/g) || []).filter(
      (m) => !/:\s*0(?:\s*!important)?\s*$/.test(m),
    )
    expect(strayMinHeight, `多余的 min-height: ${strayMinHeight.join(';')}`).toEqual([])
  })

  it('uses dynamic viewport unit (100dvh) so mobile address bar does not break layout', () => {
    expect(NAV_SRC).toMatch(/100dvh/)
  })

  it('.app-content is display:flex (column) so page-content can flex:1 fill (Stage 105 browser 实测)', () => {
    // Stage 105 dev 模式浏览器实测: chat-main 高度仅 332px, pageContent 632px,
    // conversation-page 没撑满, composer 不贴底.
    // 根因: .app-content 是 block, 不参与 flex, 导致 app-header 不挤压 page-content
    const block = firstBlock('.app-content')
    expect(block, '.app-content 块未找到').not.toBe('')
    expect(block).toMatch(/display\s*:\s*flex/)
    expect(block).toMatch(/flex-direction\s*:\s*column/)
    // flex 子级 (app-header/page-content) 需要 min-height:0 才能让 page-content 收缩/撑满
    expect(block).toMatch(/min-height\s*:\s*0/)
  })

  // E2E-11：nav 布局必须挂载 NotifyHost，否则聊天区所有 notify() 都是静默的
  //
  // 浏览器实测（2026-09-19）：/chat/user 用 nav 布局，触发 notify()（表单校验失败、
  // 保存成功、头像上传成功/失败、退出登录）后 DOM 里 `.notify-stack` / `.notify-card`
  // 均为 0，页面无任何反馈。而 NotifyHost 此前只挂在 layouts/default.vue 上。
  it('nav 布局必须渲染 NotifyHost（否则聊天区所有 toast 静默）', () => {
    expect(NAV_SRC).toMatch(/<NotifyHost\s*\/>/)
  })
})
