// app/pages/chat/agents.contract.test.ts
//
// CL-06 · Phase 7：角色管理页的**静态契约**（页面行为细节由 utils/agentForm.test.ts 覆盖）。
//
// 为什么用源码契约而不是挂载测试：本页依赖 Nuxt 页面级 auto-import 与布局上下文，
// 挂载成本高、收益低；而这里要钉的恰好是"页面**必须引用**哪些能力"——
// 少了哪一条就是一处功能缺口（本仓的 `shouldGenerateTitle` 就是"字段没接住"的教训）。
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const PAGE = readFileSync(resolve(process.cwd(), 'app/pages/chat/agents.vue'), 'utf8')
const NAV = readFileSync(resolve(process.cwd(), 'app/layouts/nav.vue'), 'utf8')

describe('角色管理页 · 能力契约', () => {
  it('用 store 的能力（列表 / 保存 / 删除 / 上限），不自己拼 HTTP', () => {
    expect(PAGE).toMatch(/useAgentStore/)
    expect(PAGE).toMatch(/agentStore\.fetchAgents/)
    expect(PAGE).toMatch(/agentStore\.saveAgent/)
    expect(PAGE).toMatch(/agentStore\.removeAgent/)
    expect(PAGE).toMatch(/agentStore\.canCreateMore/)
    expect(PAGE).not.toMatch(/fetch\(/) // 页面不得绕过 store/useApi 直连
  })

  it('表单校验与预览走纯函数（utils/agentForm），不在模板里重造一份', () => {
    expect(PAGE).toMatch(/validateAgentForm/)
    expect(PAGE).toMatch(/buildAgentPreview/)
    expect(PAGE).toMatch(/AGENT_LIMITS/)
    expect(PAGE).toMatch(/VOICE_OPTIONS/)
  })

  it('达到上限时禁用「新建角色」并给出原因（不让用户点了没反应）', () => {
    expect(PAGE).toMatch(/:disabled="!agentStore\.canCreateMore"/)
    expect(PAGE).toMatch(/已达上限/)
  })

  it('页面存在且是 laravel 风格目录结构下的路由页（/chat/agents）', () => {
    expect(NAV).toMatch(/\/chat\/agents/)
    expect(NAV).toMatch(/label: '角色'/)
  })

  it('保存失败时把服务端消息展示出来（含内容黑名单的 field/category）', () => {
    expect(PAGE).toMatch(/agentStore\.error/)
    expect(PAGE).toMatch(/agent-errors/)
  })

  // UI 轮（用户 2026-10-18：内联面板"逻辑很怪，不如出现一个弹窗"）：
  // 编辑器改成**模态弹窗** —— 关键不是"画个框"，而是模态该有的四件事必须有：
  // 语义（role=dialog + aria-modal）、Esc 关闭、点遮罩关闭、打开时锁背景滚动。
  // 少了任何一条，它就是个"看起来像弹窗、用起来不是"的壳子。
  it('编辑器是真正的模态弹窗（语义 + Esc + 遮罩 + 滚动锁）', () => {
    expect(PAGE).toMatch(/role="dialog"/)
    expect(PAGE).toMatch(/aria-modal="true"/)
    expect(PAGE).toMatch(/aria-labelledby=/)
    expect(PAGE, 'Esc 必须能关（用 document 级监听，而不是只挂在面板上）').toMatch(
      /addEventListener\(\s*['"]keydown['"]/,
    )
    expect(PAGE).toMatch(/Escape/)
    expect(PAGE, '点遮罩关闭').toMatch(/@click\.self=/)
    expect(PAGE, '打开时锁背景滚动').toMatch(/document\.body\.style\.overflow/)
  })

  it('关闭弹窗时对"未保存的修改"有交代（不静默丢弃用户写的字）', () => {
    expect(PAGE).toMatch(/discardConfirm/)
    expect(PAGE).toMatch(/放弃/)
  })

  // 第 3 条反馈：原生 <select> 展开层由浏览器绘制、不在 DOM 里 ⇒ CSS 改不动，
  // 必须换成自绘下拉（EeSelect，键盘/点外面/Esc 的行为由它自己的测试钉住）。
  it('音色用自绘下拉，不再用原生 select', () => {
    expect(PAGE).toMatch(/<EeSelect/)
    expect(PAGE, '数据仍来自 VOICE_OPTIONS（不许在页面里重抄一份音色表）').toMatch(/VOICE_OPTIONS/)

    // 只在**模板**里找原生 select 元素，并先剥掉 HTML 注释 —— 否则解释这件事的注释
    // （"原生 <select> 换成自绘…"）会把门禁误报成缺陷。本仓反复踩过这个坑。
    const template = PAGE.slice(PAGE.indexOf('<template>'), PAGE.indexOf('</template>'))
    expect(template.replace(/<!--[\s\S]*?-->/g, '')).not.toMatch(/<select[\s>]/)
  })

  // UI 轮：主按钮的文字色必须是令牌。写死 `#fff` 在深色模式下会压在**浅**鼠尾草绿
  // （--ee-primary = #8ebaa8）上，实测对比度约 1.9:1（AA 要 4.5:1）⇒ 几乎读不出来。
  it('主按钮文字用 --ee-on-primary（不写死 #fff —— 深色模式下会白字压浅底）', () => {
    expect(PAGE).toMatch(/\.primary-btn[\s\S]{0,200}?color:\s*var\(--ee-on-primary\)/)
    expect(PAGE).not.toMatch(/\.primary-btn[\s\S]{0,200}?color:\s*#fff/i)
  })
})
