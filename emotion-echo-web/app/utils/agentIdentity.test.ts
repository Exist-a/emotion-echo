// app/utils/agentIdentity.test.ts
//
// CL-06 · UI 轮：角色卡"身份色"与"首字"的**纯函数**契约。
//
// 为什么单列一个 util：一个用户最多 5 个角色，卡片并排时最能说明"这是好几个不同的人"的
// 就是那枚圆形徽记。它的颜色必须**由名字决定且稳定**（刷新不变、换设备不变），
// 否则每次重渲染都在换色 —— 那正是"看着像随机装饰"的来源。
// 色调的**取值域**刻意只有 4 个且全部来自项目调色板（见 AgentCard.vue 的 tone-N），
// 所以这里只钉"确定性 + 落在值域内"，不钉具体色值（色值属于样式，改色不该挂测试）。
import { describe, it, expect } from 'vitest'
import { AGENT_TONE_COUNT, agentInitial, agentToneIndex } from './agentIdentity'

describe('agentIdentity · 首字徽记', () => {
  it('取名字第一个字符', () => {
    expect(agentInitial('小E')).toBe('小')
    expect(agentInitial('阿哲')).toBe('阿')
  })

  it('名字为空/纯空白 ⇒ 回落占位字，不渲染空圆圈', () => {
    expect(agentInitial('')).toBe('角')
    expect(agentInitial('   ')).toBe('角')
    expect(agentInitial(undefined)).toBe('角')
    expect(agentInitial(null)).toBe('角')
  })

  it('不按 UTF-16 码元切（emoji / 代理对不会被劈成半个字符）', () => {
    expect(agentInitial('🌱小青')).toBe('🌱')
  })
})

describe('agentIdentity · 身份色调', () => {
  it('同一名字恒定落在同一色调（刷新不变）', () => {
    expect(agentToneIndex('小E')).toBe(agentToneIndex('小E'))
    expect(agentToneIndex('阿哲')).toBe(agentToneIndex('阿哲'))
  })

  it('落到 [0, AGENT_TONE_COUNT) 内（样式类名 tone-N 一定有对应的 N）', () => {
    const names = ['小E', '阿哲', '树洞', 'a', '某个很长的角色名字', '🌱', '']
    for (const n of names) {
      const i = agentToneIndex(n)
      expect(Number.isInteger(i)).toBe(true)
      expect(i).toBeGreaterThanOrEqual(0)
      expect(i).toBeLessThan(AGENT_TONE_COUNT)
    }
  })

  it('不同名字能落到不同色调（不是恒等于 0 的假实现）', () => {
    const distinct = new Set(['小E', '阿哲', '树洞'].map(agentToneIndex))
    expect(distinct.size).toBeGreaterThan(1)
  })

  it('空名字也有稳定色调（预览态还没填名字时不能崩）', () => {
    expect(agentToneIndex('')).toBe(agentToneIndex(''))
    expect(() => agentToneIndex(undefined)).not.toThrow()
  })
})
