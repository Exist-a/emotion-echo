import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const QUESTION_SRC = readFileSync(resolve(__dirname, 'index.vue'), 'utf8')
const NAV_LINKS = [
  '/chat/conversation', // 对话
  '/question', // 心理测验
  '/chat/dashboard/dailyReport', // 日报
  '/chat/dashboard/weeklyReport',
  '/chat/dashboard/monthlyReport',
  '/chat/dashboard/annualReport',
  '/chat/user', // 我的空间
  '/chat/setting', // 设置
]

// Stage 112 修复：question 页之前用 layout: 'default'，导致 sidebar 不可见，
// 用户点 sidebar "心理测验" 链接后整页失去导航，只能靠页面里的"回到我的空间"回主页。
// 本测试钉死契约：question 页必须用 layout: 'nav'，且侧边栏入口必须可达。
describe('question page layout contract (Stage 112 bug B)', () => {
  it('question/index.vue 使用 layout: "nav"（不是 default），让 sidebar 在心理测验页可见', () => {
    expect(QUESTION_SRC).toMatch(/definePageMeta\(\s*\{[^}]*layout\s*:\s*['"]nav['"]/)
    // 反向断言：不能误用 default
    expect(QUESTION_SRC).not.toMatch(/layout\s*:\s*['"]default['"]/)
  })

  it('nav.vue 的 primaryLinks 必须含 /question 入口，且 sidebar 链接 href 覆盖 8 个分区', () => {
    const NAV_SRC = readFileSync(resolve(__dirname, '../../layouts/nav.vue'), 'utf8')
    for (const href of NAV_LINKS) {
      expect(NAV_SRC, `nav.vue 应含 sidebar 入口 ${href}`).toContain(href)
    }
  })
})
