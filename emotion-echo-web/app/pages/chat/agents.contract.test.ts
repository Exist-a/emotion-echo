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
})
