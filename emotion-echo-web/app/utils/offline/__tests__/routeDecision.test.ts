/**
 * routeDecision 字面量契约测试（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：v0.2 §5.1 路由决策树纯函数，注入 input 与上下文，返回端/云/兜底路由。
 *
 * 决策表（来自 v0.2 §5.1）：
 *   1. 设备不支持 WebGPU            → cloud
 *   2. 端侧模型未加载完成           → cloud
 *   3. 命中一级高危 + 联网           → cloud
 *   4. 命中一级高危 + 离线           → fallback_hotline（兜住"高危×断网"交叉格）
 *   5. 输入文本 >500 字              → cloud
 *   6. 离线状态（前提：上述条件不命中） → local
 *   7. 其他                          → local
 */
import { describe, expect, it } from 'vitest'
import {
  decideRoute,
  HIGH_RISK_TOKENS,
  HIGH_RISK_HOTLINE_TEMPLATE,
  LONG_INPUT_THRESHOLD,
  type RouteContext,
  type RouteDecision,
} from '../routeDecision'

/** 测试夹具：构造合法 RouteContext */
function ctx(over: Partial<RouteContext> = {}): RouteContext {
  return {
    deviceCap: 'webgpu',
    modelLoaded: true,
    online: true,
    input: '今天上班有点累',
    layer: 'daily',
    ...over,
  }
}

describe('HIGH_RISK_TOKENS 常量', () => {
  it('必须包含"自杀"/"自残"/"不想活"', () => {
    expect(HIGH_RISK_TOKENS).toContain('自杀')
    expect(HIGH_RISK_TOKENS).toContain('自残')
    expect(HIGH_RISK_TOKENS).toContain('不想活')
  })
})

describe('HIGH_RISK_HOTLINE_TEMPLATE', () => {
  it('必须含热线号码 400-161-9995 + 固定危机回应', () => {
    expect(HIGH_RISK_HOTLINE_TEMPLATE).toContain('400-161-9995')
    expect(HIGH_RISK_HOTLINE_TEMPLATE.length).toBeGreaterThan(20)
  })
})

describe('LONG_INPUT_THRESHOLD', () => {
  it('必须 = 500（v0.2 §5.1 阈值）', () => {
    expect(LONG_INPUT_THRESHOLD).toBe(500)
  })
})

describe('decideRoute — 7 个决策分支', () => {
  it('分支 1: 设备不支持 → cloud', () => {
    const d = decideRoute(ctx({ deviceCap: 'unsupported' }))
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/设备/)
  })

  it('分支 1b: 软件回退（software）也走 cloud（语义同上：非真 WebGPU）', () => {
    const d = decideRoute(ctx({ deviceCap: 'software' }))
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/设备/)
  })

  it('分支 2: 模型未加载 → cloud', () => {
    const d = decideRoute(ctx({ modelLoaded: false }))
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/未加载/)
  })

  it('分支 3: 高危 + 联网 → cloud（让云端专业处理）', () => {
    const d = decideRoute(
      ctx({ online: true, input: '我不想活了，怎么办' })
    )
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/高危/)
  })

  it('分支 4: 高危 + 离线 → fallback_hotline（兜底，不依赖模型/网络）', () => {
    const d = decideRoute(
      ctx({ online: false, input: '最近总想自杀' })
    )
    expect(d.route).toBe('fallback_hotline')
    expect(d.reason).toMatch(/高危/)
    expect(d.template).toBe(HIGH_RISK_HOTLINE_TEMPLATE)
  })

  it('分支 5: 超长输入（>500 字）→ cloud', () => {
    const longInput = '啊'.repeat(LONG_INPUT_THRESHOLD + 1)
    const d = decideRoute(ctx({ input: longInput }))
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/超长/)
  })

  it('分支 5 边界: 恰好 500 字 → 不算超长（默认 local）', () => {
    const exact500 = '啊'.repeat(LONG_INPUT_THRESHOLD)
    const d = decideRoute(ctx({ input: exact500 }))
    expect(d.route).toBe('local')
  })

  it('分支 6: 离线 + 普通输入 → local', () => {
    const d = decideRoute(ctx({ online: false, input: '今天心情不太好' }))
    expect(d.route).toBe('local')
    expect(d.reason).toMatch(/离线/)
  })

  it('分支 7: 默认情况 → local', () => {
    const d = decideRoute(ctx())
    expect(d.route).toBe('local')
    expect(d.reason).toMatch(/端侧/)
  })

  it('优先级：高危优先于"设备不支持"——先弹危机回应', () => {
    // 即便设备不支持，也应优先响应危机输入（仍可走 cloud 转人工）
    const d = decideRoute(
      ctx({ deviceCap: 'unsupported', input: '我想结束生命' })
    )
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/高危/)
  })

  it('优先级：高危优先于"超长"——危机不需要长文', () => {
    const long = '想死啊'.repeat(200) // 600 字 + 含高危
    const d = decideRoute(ctx({ input: long }))
    expect(d.route).toBe('cloud')
    expect(d.reason).toMatch(/高危/)
  })

  it('decideRoute 必须返回合法 RouteDecision 形状', () => {
    const allowed: RouteDecision['route'][] = [
      'local',
      'cloud',
      'fallback_hotline',
    ]
    for (const r of [decideRoute(ctx()), decideRoute(ctx({ online: false }))]) {
      expect(allowed).toContain(r.route)
      expect(typeof r.reason).toBe('string')
      expect(r.reason.length).toBeGreaterThan(0)
    }
  })

  it('高危 token 变体：包含空格/同义词都应识别', () => {
    // "不想 活" 含空格（用户输入可能有标点）
    const d = decideRoute(ctx({ input: '我今天觉得不想 活了，怎么办' }))
    expect(d.route).toBe('cloud')
  })
})