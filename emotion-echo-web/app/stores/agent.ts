// app/stores/agent.ts
//
// CL-06（用户自定义 Agent·多角色）：角色档案的前端状态与读写。
//
// 职责边界（刻意画清）：
//   · 本 store 只管"取/存/删 + 选谁"，**不做安全判定**（字段上限、内容黑名单、
//     每户 ≤5 的强制都在服务端 —— 前端校验只是早提示，绕不过 curl）；
//   · `resolveActiveAgent` 与 BFF `resolveAgentProfile` **同语义**（指定 → 默认 → 首项 → null），
//     但用途不同：前端只决定"界面显示谁 / 发消息时带哪个 id"，**真正注入谁由服务端判定**
//     （客户端传的 id 会在服务端与自己列表比对，不存在越权）。
import { defineStore } from 'pinia'
import type { AgentProfile, AgentProfilePayload } from '~/types/api'
import { get, post, del } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'

/** 每户角色上限（与 user-svc `MaxAgentProfilesPerUser` 一致；前端仅用于禁用"新增"按钮） */
export const MAX_AGENT_PROFILES = 5

export const useAgentStore = defineStore('agent', () => {
  const agents = ref<AgentProfile[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** 当前打开会话所选角色 id（0 = 未选 ⇒ 界面显示默认角色） */
  const activeAgentId = ref<number>(0)

  const canCreateMore = computed(() => agents.value.length < MAX_AGENT_PROFILES)

  const defaultAgent = computed<AgentProfile | null>(() => {
    if (agents.value.length === 0) return null
    return agents.value.find((a) => a.isDefault) ?? agents.value[0]!
  })

  /**
   * 解析"本会话实际生效的角色"。
   *
   * 回落链（与 BFF 同款，见 `ai_stream_handler.go: resolveAgentProfile`）：
   * 指定 id 命中 → 用户默认角色 → 列表首项 → null（无角色 ⇒ 不显示人设、不盖角色戳）。
   */
  const resolveActiveAgent = (requestedId?: number): AgentProfile | null => {
    if (agents.value.length === 0) return null
    const wanted = Number(requestedId ?? 0)
    if (wanted > 0) {
      const hit = agents.value.find((a) => a.id === wanted)
      if (hit) return hit
    }
    return defaultAgent.value
  }

  const activeAgent = computed<AgentProfile | null>(() =>
    resolveActiveAgent(activeAgentId.value),
  )

  const setActiveAgentId = (id: number) => {
    activeAgentId.value = Number.isFinite(id) ? id : 0
  }

  /** 拉列表（含服务端 get-or-create：首次读会物化默认「小E」）。失败不抛，只记 error。 */
  const fetchAgents = async (): Promise<AgentProfile[]> => {
    loading.value = true
    error.value = null
    try {
      const data = await get<{ agents: AgentProfile[]; total: number }>(API_ROUTES.agents.path)
      agents.value = data?.agents ?? []
      return agents.value
    } catch (e) {
      // 增强不是依赖：取不到角色列表时对话仍要能用（回落"无角色"路径）
      error.value = e instanceof Error ? e.message : '角色列表加载失败'
      agents.value = []
      return []
    } finally {
      loading.value = false
    }
  }

  /** 新建（payload.id 为空）或更新（payload.id > 0）。成功回带保存结果，失败返回 null。 */
  const saveAgent = async (payload: AgentProfilePayload): Promise<AgentProfile | null> => {
    loading.value = true
    error.value = null
    try {
      const body: Record<string, unknown> = { ...payload }
      if (!body.id) delete body.id // 0/undefined ⇒ 让服务端走"新建"
      const saved = await post<AgentProfile>(API_ROUTES.agentUpsert.path, body)
      await fetchAgents()
      return saved
    } catch (e) {
      error.value = e instanceof Error ? e.message : '保存失败'
      return null
    } finally {
      loading.value = false
    }
  }

  /** 软删除（幂等）。成功返回 true 并刷新列表。 */
  const removeAgent = async (id: number): Promise<boolean> => {
    loading.value = true
    error.value = null
    try {
      await del(API_ROUTES.agentById.path.replace(':id', String(id)))
      await fetchAgents()
      // 被删的恰好是当前选中 ⇒ 回到"未选"（界面自动落到默认角色）
      if (activeAgentId.value === id) activeAgentId.value = 0
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : '删除失败'
      return false
    } finally {
      loading.value = false
    }
  }

  const clearError = () => {
    error.value = null
  }

  return {
    agents,
    loading,
    error,
    activeAgentId,
    activeAgent,
    defaultAgent,
    canCreateMore,
    resolveActiveAgent,
    setActiveAgentId,
    fetchAgents,
    saveAgent,
    removeAgent,
    clearError,
  }
})
