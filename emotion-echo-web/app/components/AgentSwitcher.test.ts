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

  // UI 轮（用户 2026-10-18 反馈"切换角色的容器被压在下面了，应该在最上面"）：
  // composer 贴近页面底部、菜单又向上展开 ⇒ 它必然叠在消息区上方，必须显式声明层级。
  // 两处都要：**容器**自身建层叠上下文（整块控件压在消息区之上），
  // **菜单**再给更高的值（压住自己的兄弟节点）。只写菜单一处是不够的 —— 若外层
  // 参与了别的层叠上下文，菜单的 z-index 只在那一层里比较，仍会被整体压住。
  it('容器与菜单都必须显式抬高层级（否则会被消息区压住）', () => {
    const switcherRule = src.match(/\.agent-switcher\s*\{[\s\S]*?\n\}/)
    expect(switcherRule, '应能找到 .agent-switcher 规则').not.toBeNull()
    expect(
      Number((switcherRule![0].match(/z-index:\s*(\d+)/) ?? [])[1]),
      '.agent-switcher 需要 z-index 建立层叠上下文（"容器被压在下面"就是缺了这一步）',
    ).toBeGreaterThanOrEqual(100)

    const menuRule = src.match(/\.agent-menu\s*\{[\s\S]*?\n\}/)
    expect(menuRule, '应能找到 .agent-menu 规则').not.toBeNull()
    expect(
      Number((menuRule![0].match(/z-index:\s*(\d+)/) ?? [])[1]),
      '.agent-menu 的 z-index 必须高于其容器',
    ).toBeGreaterThanOrEqual(1000)
  })

  // 用户 2026-10-18 **第二次**指出同一处（并附了元素选择）："管理角色这个选择框在另一个元素下面"。
  //
  // 实测纠正（逐点命中测试，不是看样式猜）：菜单**本身在最上层**，并没有被任何元素盖住
  // —— 之前我按"下一层是谁"读出了错误的"遮挡者"清单。真正的问题是它被**祖先裁掉**：
  // `conversation-page` 有 `overflow: hidden`（盒子右边缘 x=1229），而菜单以 `left: 0`
  // 从触发器**左**边缘向右展开（触发器 x 1064..1119，菜单 190px 宽 ⇒ 到 1254）
  // ⇒ **右侧 25px 被裁**，被裁处露出来的正是 `div.page-content`
  // （x=1232 起命中 page-content，正是用户选中的第二个元素）。
  //
  // 修法：改贴触发器**右**边缘向左展开（`right: 0`）—— 菜单整体落进祖先盒子内，
  // 不再越界，也就不可能被裁。行为钉在 `e2e/cl-06-custom-agent.spec.ts`（按坐标命中断言）。
  it('菜单贴触发器**右边缘**向左展开（左对齐会顶出祖先 overflow 被裁掉右侧）', () => {
    const menuRule = src.match(/\.agent-menu\s*\{[\s\S]*?\n\}/)
    expect(menuRule, '应能找到 .agent-menu 规则').not.toBeNull()
    expect(menuRule![0], '必须以右边缘为锚（right: 0）').toMatch(/right:\s*0/)
    expect(menuRule![0], '不得再以左边缘为锚（left: 0 会向右顶出容器右边缘）').not.toMatch(/\bleft:\s*0/)
    expect(menuRule![0], '仍须向上展开').toMatch(/bottom:\s*100%/)
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
