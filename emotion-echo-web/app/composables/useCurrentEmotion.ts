// composables/useCurrentEmotion.ts
//
// CL-07 前端半（情绪回显）· Phase 2：把"融合情绪"取回来并转成**可安全展示**的状态。
//
// 背景：`fused_emotions`（三模态收敛后的综合情绪）此前前端**零消费** —— 顶部状态条、
// 消息角标、数字人表情都没有真值。本 composable 负责取数 + 降级 + 护栏文案。
//
// 纪律（沿用 D-14 三句护栏 / 画像注入同款「增强非依赖」）：
//   - 任何失败（404/503/网络/字段缺失）⇒ 静默降级为"不显示"，**不阻塞主对话、不抛错**；
//   - 文案陈述化、中性，**不得出现来源词**（识别/检测/摄像头/分析/监控）；
//   - 无 face/voice 模态时**不得**展示成"多模态"（用 `modalityLabel` 降级为「文本」）。

import { API_ROUTES } from '~/lib/apiRoutes'
import { useApi } from '~/composables/useApi'
import { getEmotionLabel } from '~/utils/emotion'

/** BFF `/emotion/message/:id/fused` → `{fused: FusedEmotion}`（Go proto 的 json tag 为 snake_case） */
export interface FusedEmotionDto {
  primary_emotion?: string
  confidence?: number
  fusion_method?: string
  /** JSON **字符串**数组（如 `"[\"text\"]"`）——注意不是数组本身 */
  available_modalities?: string
}

export interface CurrentEmotionState {
  /** 融合主情绪（如 happy / sad / neutral） */
  emotion: string
  /** 中文标签 */
  label: string
  /** 展示文案（护栏：陈述化、中性、无来源词） */
  text: string
  /** 模态展示标签：含 face/voice → 「综合」；否则 → 「文本」 */
  modalityLabel: '综合' | '文本'
  hasMultiModal: boolean
}

/** 判定"真多模态"的模态键（text 不算） */
const MULTIMODAL_KEYS = ['face', 'voice'] as const

/** 护栏禁用词（来源词）——出现即视为护栏失守 */
export const FORBIDDEN_SOURCE_WORDS = ['识别', '检测', '摄像头', '分析', '监控'] as const

/** 容忍非法输入地把 `available_modalities`（JSON 字符串）解析成数组 */
export function parseModalities(raw?: string): string[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return []
  }
}

/** 是否真的用到了 face/voice 模态（纯 text 不算多模态） */
export function hasMultiModal(modalities: string[]): boolean {
  return modalities.some((m) => (MULTIMODAL_KEYS as readonly string[]).includes(m))
}

/** 护栏文案：陈述化 + 中性（"此刻的状态：开心"），不带来源词 */
export function buildEmotionText(emotion: string): string {
  return `此刻的状态：${getEmotionLabel(emotion)}`
}

/** FusedEmotion DTO → 展示状态；无有效情绪时返回 null（调用方据此隐藏） */
export function toCurrentEmotionState(f?: FusedEmotionDto | null): CurrentEmotionState | null {
  const emotion = (f?.primary_emotion || '').trim()
  if (!emotion) return null
  const mm = hasMultiModal(parseModalities(f?.available_modalities))
  return {
    emotion,
    label: getEmotionLabel(emotion),
    text: buildEmotionText(emotion),
    modalityLabel: mm ? '综合' : '文本',
    hasMultiModal: mm,
  }
}

/** 取数器：默认走 useApi；测试可注入替身 */
export type FusedEmotionFetcher = (messageId: string) => Promise<FusedEmotionDto | null>

function defaultFetcher(): FusedEmotionFetcher {
  return async (messageId: string) => {
    const { get } = useApi()
    const res = await get<{ fused?: FusedEmotionDto }>(
      API_ROUTES.emotionFusedByMessage.path.replace(':messageId', messageId),
    )
    return res?.fused ?? null
  }
}

export function useCurrentEmotion(fetcher?: FusedEmotionFetcher) {
  const current = ref<CurrentEmotionState | null>(null)
  const loading = ref(false)
  /** 消息级缓存：messageId → 状态（null = 确认无融合，避免重复拉取） */
  const byMessage = ref<Record<string, CurrentEmotionState | null>>({})
  const fetchFused = fetcher ?? defaultFetcher()

  /**
   * 拉取某条消息的融合情绪并更新 `current`。
   * - messageId 为空 ⇒ 清空（不请求）
   * - 任何异常/无数据 ⇒ 清空（**不抛出**：情绪回显是增强不是依赖）
   */
  async function refreshForMessage(messageId?: string | number | null): Promise<void> {
    const id = messageId == null ? '' : String(messageId).trim()
    if (!id) {
      current.value = null
      return
    }
    loading.value = true
    try {
      const fused = await fetchFused(id)
      current.value = toCurrentEmotionState(fused)
    } catch {
      current.value = null
    } finally {
      loading.value = false
    }
  }

  /**
   * 批量补齐消息级情绪（气泡角标用）。
   *
   * 防放大：① 只取**最新 cap 条**；② 已缓存的（含"确认无融合"）不重复拉；
   * ③ 失败也写入 `null` 缓存，避免每轮渲染反复打请求（计划 §6 的 N+1 风险）。
   */
  async function ensureMessages(messageIds: (string | number)[], cap = 12): Promise<void> {
    const targets = messageIds
      .map((v) => String(v ?? '').trim())
      .filter((id) => id.length > 0)
      .slice(-cap)
      .filter((id) => !(id in byMessage.value))
    if (targets.length === 0) return
    await Promise.all(
      targets.map(async (id) => {
        let state: CurrentEmotionState | null = null
        try {
          state = toCurrentEmotionState(await fetchFused(id))
        } catch {
          state = null
        }
        byMessage.value = { ...byMessage.value, [id]: state }
      }),
    )
  }

  function clear(): void {
    current.value = null
    byMessage.value = {}
  }

  return { current, loading, byMessage, refreshForMessage, ensureMessages, clear }
}
