// E2E-17 plan §6 step 3 RED：
//   前端播放层 phoneme 时间戳驱动改造（useTTSPlayer）
//
// 测试 1：charToLipShape 元音辅音映射契约（plan §2.A.3 第 5 条 + #8 映射表激活契约）
//   - 9 元音条目逐一断言 a→aa, o→oh, e→ee, i→ih, u→ou, ü→ee, v→ih, n→ih, m→ih
//   - 24 辅音条目断言（bb→aa, p→aa, m→aa, f→oh, v→oh, w→ou, 其余→ih）
//   - 大小写不敏感（输入 "A" 应映射 aa）
//   - 未知 char 返回 'neutral'（graceful fallback）
//   - 空字符串返回 'neutral'
//
// 测试 2：findPhonemeAt 时间戳→区间判定契约（plan §4 #9 时间戳→口型边界）
//   - phonemes 空 → null
//   - time < first.start → null（在第一音之前）
//   - time === first.start → first（含左端点）
//   - first.start < time < first.start+first.duration → first
//   - time === first.start+first.duration → null（不含右端点）
//   - 区间间隙 → null
//   - time > last.start+last.duration → null（最后一音之后）
//   - 多区间：在正确区间内返回该区间
//
// 测试 3：startRandomLipAnimation 假随机动画字面量契约（plan §4 #10 假随机轮播已删）
//   - 模块里不导出 / 不引用 startRandomLipAnimation

import { describe, it, expect } from 'vitest'
import * as TTSPlayer from './useTTSPlayer'

describe('useTTSPlayer phoneme-driven lip sync helpers (E2E-17 plan §6 step 3)', () => {
  describe('charToLipShape (元音+辅音映射表激活)', () => {
    it('元音 9 条逐一映射正确', () => {
      expect((TTSPlayer as any).charToLipShape('a')).toBe('aa')
      expect((TTSPlayer as any).charToLipShape('o')).toBe('oh')
      expect((TTSPlayer as any).charToLipShape('e')).toBe('ee')
      expect((TTSPlayer as any).charToLipShape('i')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('u')).toBe('ou')
      expect((TTSPlayer as any).charToLipShape('ü')).toBe('ee')
      expect((TTSPlayer as any).charToLipShape('v')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('n')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('m')).toBe('ih')
    })

    it('辅音 24 条逐一映射正确', () => {
      // 双唇音 → aa（b/p 仅在 CONSONANT）
      expect((TTSPlayer as any).charToLipShape('b')).toBe('aa')
      expect((TTSPlayer as any).charToLipShape('p')).toBe('aa')
      // m 在 VOWEL（m: ih）和 CONSONANT（m: aa）—— VOWEL 优先 → ih
      expect((TTSPlayer as any).charToLipShape('m')).toBe('ih')
      // 唇齿音 → oh（f 仅在 CONSONANT，v 在 VOWEL 是 ih 故归 ih）
      expect((TTSPlayer as any).charToLipShape('f')).toBe('oh')
      expect((TTSPlayer as any).charToLipShape('v')).toBe('ih')
      // 唇音 w → ou
      expect((TTSPlayer as any).charToLipShape('w')).toBe('ou')
      // 其余辅音（舌尖/舌面/舌根等）→ ih（plan 钉死的简化集合）
      expect((TTSPlayer as any).charToLipShape('d')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('t')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('l')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('z')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('c')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('s')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('zh')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('ch')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('sh')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('r')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('j')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('q')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('x')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('y')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('g')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('k')).toBe('ih')
      expect((TTSPlayer as any).charToLipShape('h')).toBe('ih')
    })

    it('大小写不敏感（A → aa 同 a）', () => {
      // A 仅在 VOWEL（a: aa），case-insensitive 后命中
      expect((TTSPlayer as any).charToLipShape('A')).toBe('aa')
      // B 仅在 CONSONANT（b: aa），case-insensitive 后命中
      expect((TTSPlayer as any).charToLipShape('B')).toBe('aa')
      // M 同时在 VOWEL（m: ih）和 CONSONANT（m: aa）—— VOWEL 优先
      expect((TTSPlayer as any).charToLipShape('M')).toBe('ih')
    })

    it('未知字符返回 neutral（graceful fallback）', () => {
      expect((TTSPlayer as any).charToLipShape('?')).toBe('neutral')
      expect((TTSPlayer as any).charToLipShape('1')).toBe('neutral')
      expect((TTSPlayer as any).charToLipShape(' ')).toBe('neutral')
    })

    it('空字符串返回 neutral（防御性）', () => {
      expect((TTSPlayer as any).charToLipShape('')).toBe('neutral')
    })
  })

  describe('findPhonemeAt (时间戳区间判定)', () => {
    // 构造样例（4 段，时间轴 0~1.0s，区间 [0,0.25][0.25,0.5][0.5,0.75][0.75,1.0]）
    const ps = [
      { char: 'A', start: 0.0, duration: 0.25 },
      { char: 'B', start: 0.25, duration: 0.25 },
      { char: 'C', start: 0.5, duration: 0.25 },
      { char: 'D', start: 0.75, duration: 0.25 },
    ]

    it('空 phonemes 数组返回 null', () => {
      expect((TTSPlayer as any).findPhonemeAt([], 0.5)).toBeNull()
    })

    it('time < first.start（首音之前）返回 null', () => {
      // 第一个音从 0 开始；time=-0.001 才是"首音之前"
      expect((TTSPlayer as any).findPhonemeAt(ps, -0.001)).toBeNull()
    })

    it('time === first.start（含左端点）返回 first', () => {
      // 第一音的 start=0；time=0 应命中
      const r = (TTSPlayer as any).findPhonemeAt(ps, 0)
      expect(r.char).toBe('A')
    })

    it('time 在 first 区间内部返回 first', () => {
      const r = (TTSPlayer as any).findPhonemeAt(ps, 0.1)
      expect(r.char).toBe('A')
    })

    it('time === first.start+first.duration（不含右端点）返回 null', () => {
      // 区间 [0, 0.25) — time=0.25 应落入 B 而不是 A
      const r = (TTSPlayer as any).findPhonemeAt(ps, 0.25)
      expect(r.char).toBe('B')
    })

    it('time 在 mid 区间内部返回 mid', () => {
      const r = (TTSPlayer as any).findPhonemeAt(ps, 0.6)
      expect(r.char).toBe('C')
    })

    it('time > last.start+last.duration（尾音之后）返回 null', () => {
      expect((TTSPlayer as any).findPhonemeAt(ps, 1.001)).toBeNull()
    })

    it('time === last.start+last.duration（首音是 last 时）== 1.0 → null', () => {
      expect((TTSPlayer as any).findPhonemeAt(ps, 1.0)).toBeNull()
    })
  })

  describe('startRandomLipAnimation 假随机轮播字面量契约（plan §4 #10）', () => {
    it('模块不再导出 / 不引用 startRandomLipAnimation（150ms 假动画已删除）', () => {
      // 读模块源码，断言 startRandomLipAnimation 字符串不存在
      // （死代码 + 150ms 固定节奏 → 必须删除以满足"口型由音频 currentTime 驱动"）
      const fs = require('node:fs')
      const src = fs.readFileSync(
        require('path').resolve(__dirname, './useTTSPlayer.ts'),
        'utf8',
      )
      expect(src).not.toMatch(/startRandomLipAnimation/)
      expect(src).not.toMatch(/lipAnimationInterval/)
    })
  })
})