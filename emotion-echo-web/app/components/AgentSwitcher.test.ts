// app/components/AgentSwitcher.test.ts
//
// CL-06 · Phase 7：composer 角色切换器契约。
//
// 用户口径（2026-10-17 两次澄清）决定了两条**位置/形态**硬约束：
//   · 位置 —— 紧邻**语音按钮左侧**（不是设置页，也不是"建会话时选一次"）；
//   · 形态 —— **向上展开**的下拉（composer 在页面底部，向下展开会顶出视口）。
// 另有两条语义约束：
//   · 选中即**写本会话**（PATCH /conversations/:id { agentId }）⇒ 中途可随时切；
//   · 底部有「管理角色」入口（去管理页），否则用户找不到增改删的地方。
//
// 第三条（本仓踩过的坑）：**切换不能打断当前输入** —— 输入框内容不受影响，
// 也不因为切角色清空草稿。
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import type { AgentProfile } from '~/types/api'

const patchMock = vi.fn()
vi.mock('~/composables/useApi', () => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: (...a: unknown[]) => patchMock(...a),
  put: vi.fn(),
  del: vi.fn(),
}))

const COMP_PATH = resolve(process.cwd(), 'app/components/AgentSwitcher.vue')

function profile(over: Partial<AgentProfile> = {}): AgentProfile {
  return {
    id: 7,
    name: '小E',
    gender: '女',
    persona: '温柔、愿意倾听的陪伴者。',
    tone: '温柔、口语化',
    boundaries: [],
    voice: 'FunAudioLLM/CosyVoice2-0.5B:anna',
    isDefault: true,
    ...over,
  }
}

async function mountSwitcher(props: Record<string, unknown> = {}) {
  const { default: AgentSwitcher } = await import('~/components/AgentSwitcher.vue')
  const wrapper = mount(AgentSwitcher, {
    props: { conversationId: '5', ...props },
    global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } },
  })
  return wrapper
}

describe('AgentSwitcher · 静态契约（位置与形态）', () => {
  const src = readFileSync(COMP_PATH, 'utf8')

  it('下拉是**向上展开**（bottom: 100%），不是向下', () => {
    expect(src).toMatch(/\.agent-menu[\s\S]{0,400}bottom:\s*100%/)
    expect(src).not.toMatch(/\.agent-menu[\s\S]{0,400}top:\s*100%/)
  })

  it('含「管理角色」入口（跳管理页）', () => {
    expect(src).toMatch(/管理角色/)
    expect(src).toMatch(/\/chat\/agents/)
  })

  it('用 aria-expanded / aria-haspopup 标注可访问性（下拉是交互控件）', () => {
    expect(src).toMatch(/aria-expanded/)
    expect(src).toMatch(/aria-haspopup/)
  })
})

describe('AgentSwitcher · 行为', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    patchMock.mockReset()
  })

  it('列出角色名（来自 store）', async () => {
    const { useAgentStore } = await import('~/stores/agent')
    useAgentStore().agents = [profile(), profile({ id: 9, name: '阿哲', isDefault: false })]

    const w = await mountSwitcher()
    // 收起时只有触发器（显示"当前生效角色"），展开后才列全部角色
    expect(w.find('.agent-trigger').text()).toContain('小E')
    expect(w.find('.agent-menu').exists()).toBe(false)

    await w.find('.agent-trigger').trigger('click')
    expect(w.text()).toContain('小E')
    expect(w.text()).toContain('阿哲')
  })

  it('点击触发器展开（初始收起）', async () => {
    const { useAgentStore } = await import('~/stores/agent')
    useAgentStore().agents = [profile(), profile({ id: 9, name: '阿哲', isDefault: false })]

    const w = await mountSwitcher()
    expect(w.find('.agent-menu').exists()).toBe(false)

    await w.find('.agent-trigger').trigger('click')
    expect(w.find('.agent-menu').exists()).toBe(true)
    expect(w.find('.agent-trigger').attributes('aria-expanded')).toBe('true')
  })

  it('选中某角色 ⇒ PATCH 本会话 agentId + 关闭下拉 + 更新本地选中态', async () => {
    patchMock.mockResolvedValue({ success: true, id: 5, agentId: 9 })
    const { useAgentStore } = await import('~/stores/agent')
    const { useConversationStore } = await import('~/stores/conversation')
    const agentStore = useAgentStore()
    agentStore.agents = [profile(), profile({ id: 9, name: '阿哲', isDefault: false })]
    useConversationStore().conversationList = [
      { id: '5', title: 't', isTop: false, userId: '1', lastMessage: null, lastMessageTime: null, createdAt: '', updatedAt: '', agentId: 7 },
    ]

    const w = await mountSwitcher()
    await w.find('.agent-trigger').trigger('click')
    await w.find('[data-agent-option="9"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))

    expect(patchMock).toHaveBeenCalledWith('/conversations/5', { agentId: 9 })
    expect(w.find('.agent-menu').exists()).toBe(false)
    expect(useConversationStore().conversationList[0]!.agentId).toBe(9)
  })

  it('切换失败 ⇒ 保留原选中并显式提示（不静默改本地态）', async () => {
    patchMock.mockRejectedValue(new Error('validation: agent profile not found for current user'))
    const { useAgentStore } = await import('~/stores/agent')
    const { useConversationStore } = await import('~/stores/conversation')
    useAgentStore().agents = [profile(), profile({ id: 9, name: '阿哲', isDefault: false })]
    useConversationStore().conversationList = [
      { id: '5', title: 't', isTop: false, userId: '1', lastMessage: null, lastMessageTime: null, createdAt: '', updatedAt: '', agentId: 7 },
    ]

    const w = await mountSwitcher()
    await w.find('.agent-trigger').trigger('click')
    await w.find('[data-agent-option="9"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))

    expect(useConversationStore().conversationList[0]!.agentId).toBe(7)
    expect(w.text()).toContain('agent profile not found')
  })

  it('当前生效角色在触发器上可见（用户知道现在是谁在陪他说话）', async () => {
    const { useAgentStore } = await import('~/stores/agent')
    const { useConversationStore } = await import('~/stores/conversation')
    useAgentStore().agents = [profile(), profile({ id: 9, name: '阿哲', isDefault: false })]
    useConversationStore().conversationList = [
      { id: '5', title: 't', isTop: false, userId: '1', lastMessage: null, lastMessageTime: null, createdAt: '', updatedAt: '', agentId: 9 },
    ]

    const w = await mountSwitcher()
    expect(w.find('.agent-trigger').text()).toContain('阿哲')
  })

  it('会话无 agentId ⇒ 触发器显示默认角色（与 BFF 回落一致）', async () => {
    const { useAgentStore } = await import('~/stores/agent')
    const { useConversationStore } = await import('~/stores/conversation')
    useAgentStore().agents = [profile(), profile({ id: 9, name: '阿哲', isDefault: false })]
    useConversationStore().conversationList = [
      { id: '5', title: 't', isTop: false, userId: '1', lastMessage: null, lastMessageTime: null, createdAt: '', updatedAt: '' },
    ]

    const w = await mountSwitcher()
    expect(w.find('.agent-trigger').text()).toContain('小E')
  })
})
