// composables/useCurrentEmotion.test.ts
//
// CL-07 前端半 · Phase 2 契约钉：取数降级 + 护栏文案 + 模态诚实降级。
//
// 重点覆盖"看起来对但其实错"的三种情形：
//   ① 纯文本被夸大成"多模态"；② 护栏文案出现来源词（识别/检测/摄像头/分析/监控）；
//   ③ 情绪回显失败反过来阻塞对话（必须静默降级、不抛错）。

import { describe, it, expect, vi } from 'vitest'
import {
  parseModalities,
  hasMultiModal,
  buildEmotionText,
  toCurrentEmotionState,
  useCurrentEmotion,
  FORBIDDEN_SOURCE_WORDS,
  type FusedEmotionDto,
} from './useCurrentEmotion'

describe('parseModalities', () => {
  it('解析 JSON 字符串数组', () => {
    expect(parseModalities('["text","face"]')).toEqual(['text', 'face'])
  })
  it('容忍空/非法 JSON/非数组（不抛错）', () => {
    expect(parseModalities(undefined)).toEqual([])
    expect(parseModalities('')).toEqual([])
    expect(parseModalities('not-json')).toEqual([])
    expect(parseModalities('{"a":1}')).toEqual([])
  })
  it('过滤非字符串元素', () => {
    expect(parseModalities('["text",1,null,"voice"]')).toEqual(['text', 'voice'])
  })
})

describe('hasMultiModal（诚实降级：纯 text 不算多模态）', () => {
  it('只有 text → false', () => {
    expect(hasMultiModal(['text'])).toBe(false)
  })
  it('含 face 或 voice → true', () => {
    expect(hasMultiModal(['text', 'face'])).toBe(true)
    expect(hasMultiModal(['voice'])).toBe(true)
  })
})

describe('护栏文案', () => {
  it('陈述化：此刻的状态：<中文标签>', () => {
    expect(buildEmotionText('happy')).toBe('此刻的状态：开心')
    expect(buildEmotionText('sad')).toBe('此刻的状态：悲伤')
  })
  it('不得出现来源词（识别/检测/摄像头/分析/监控）', () => {
    for (const e of ['happy', 'sad', 'angry', 'anxious', 'neutral', 'unknown_x']) {
      const text = buildEmotionText(e)
      for (const w of FORBIDDEN_SOURCE_WORDS) {
        expect(text).not.toContain(w)
      }
    }
  })
  it('未知情绪原样展示（不崩、不贴标签）', () => {
    expect(buildEmotionText('unknown_x')).toBe('此刻的状态：unknown_x')
  })
})

describe('toCurrentEmotionState', () => {
  it('纯文本融合 → modalityLabel=文本、hasMultiModal=false', () => {
    const f: FusedEmotionDto = {
      primary_emotion: 'neutral',
      confidence: 0.9,
      fusion_method: 'late_fusion_weighted',
      available_modalities: '["text"]',
    }
    const s = toCurrentEmotionState(f)
    expect(s).not.toBeNull()
    expect(s!.label).toBe('中性')
    expect(s!.modalityLabel).toBe('文本')
    expect(s!.hasMultiModal).toBe(false)
  })

  it('含 face 的融合 → modalityLabel=综合、hasMultiModal=true', () => {
    const s = toCurrentEmotionState({
      primary_emotion: 'happy',
      available_modalities: '["text","face"]',
    })
    expect(s!.modalityLabel).toBe('综合')
    expect(s!.hasMultiModal).toBe(true)
  })

  it('无情绪 / 空 DTO → null（调用方据此隐藏）', () => {
    expect(toCurrentEmotionState(null)).toBeNull()
    expect(toCurrentEmotionState(undefined)).toBeNull()
    expect(toCurrentEmotionState({})).toBeNull()
    expect(toCurrentEmotionState({ primary_emotion: '   ' })).toBeNull()
  })
})

describe('useCurrentEmotion 取数（注入替身，不依赖 Nuxt 运行时）', () => {
  it('有融合数据 → current 被填充', async () => {
    const fetcher = vi.fn(async () => ({
      primary_emotion: 'anxious',
      available_modalities: '["text"]',
    }))
    const { current, loading, refreshForMessage } = useCurrentEmotion(fetcher)
    await refreshForMessage(951)
    expect(fetcher).toHaveBeenCalledWith('951')
    expect(loading.value).toBe(false)
    expect(current.value?.label).toBe('焦虑')
    expect(current.value?.text).toBe('此刻的状态：焦虑')
  })

  it('无数据（404/空）→ current=null，不抛错', async () => {
    const { current, refreshForMessage } = useCurrentEmotion(async () => null)
    await refreshForMessage('1')
    expect(current.value).toBeNull()
  })

  it('取数抛错 → 静默降级为不显示（情绪回显是增强不是依赖）', async () => {
    const { current, loading, refreshForMessage } = useCurrentEmotion(async () => {
      throw new Error('boom 503')
    })
    await expect(refreshForMessage('1')).resolves.toBeUndefined()
    expect(current.value).toBeNull()
    expect(loading.value).toBe(false)
  })

  it('messageId 为空 → 清空且不请求', async () => {
    const fetcher = vi.fn(async () => ({ primary_emotion: 'happy' }))
    const { current, refreshForMessage } = useCurrentEmotion(fetcher)
    await refreshForMessage(null)
    expect(fetcher).not.toHaveBeenCalled()
    expect(current.value).toBeNull()
  })
})
