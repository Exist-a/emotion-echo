import { describe, it, expect } from 'vitest'
import { safeGet } from './safe'

describe('safeGet', () => {
  const obj = {
    a: { b: { c: 42 } },
    arr: [10, 20, 30],
    n: null,
    z: 0,
    empty: '',
  }

  it('顶层路径', () => {
    expect(safeGet(obj, 'a')).toBe(obj.a)
  })

  it('深层路径', () => {
    expect(safeGet(obj, 'a.b.c')).toBe(42)
  })

  it('路径不存在返默认值', () => {
    expect(safeGet(obj, 'a.b.x.y', 'fallback')).toBe('fallback')
  })

  it('null 中间节点返默认值', () => {
    expect(safeGet(obj, 'n.foo', 'fb')).toBe('fb')
  })

  it('数组索引访问（用 dotted 路径）', () => {
    // safeGet 仅支持 . 分隔的纯 key 路径，不支持 [n] 风格
    expect(safeGet(obj, 'arr.0')).toBe(10)
    expect(safeGet({ arr: [10] }, 'arr.0')).toBe(10)
  })

  it('数组 [n] 语法不支持（按字面路径查找）', () => {
    // 注意：实现不是 lodash.get，期望返默认值
    expect(safeGet(obj, 'arr[0]', 'fb')).toBe('fb')
  })

  it('缺失字段 undefined 返默认值', () => {
    expect(safeGet(obj, 'missing', 'fb')).toBe('fb')
  })

  it('零 / 空字符串 不是 undefined，照返', () => {
    expect(safeGet(obj, 'z', 'fb')).toBe(0)
    expect(safeGet(obj, 'empty', 'fb')).toBe('')
  })
})
