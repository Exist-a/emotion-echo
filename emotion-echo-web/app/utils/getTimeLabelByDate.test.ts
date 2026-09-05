import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { getTimeLabelByDate } from './getTimeLabelByDate'

describe('getTimeLabelByDate', () => {
  let now: number
  beforeEach(() => { now = Date.now(); vi.useFakeTimers(); vi.setSystemTime(now) })
  afterEach(() => { vi.useRealTimers() })

  it('classifies today as "今天"', () => {
    const target = new Date(now)
    expect(getTimeLabelByDate(target)).toBe('今天')
  })

  it('classifies a few hours ago as "今天"', () => {
    const target = new Date(now - 3 * 60 * 60 * 1000)
    expect(getTimeLabelByDate(target)).toBe('今天')
  })

  it('classifies yesterday as "一周内"', () => {
    const target = new Date(now - 2 * 24 * 60 * 60 * 1000)
    expect(getTimeLabelByDate(target)).toBe('一周内')
  })

  it('classifies a few days ago within seven as "一周内"', () => {
    const target = new Date(now - 6 * 24 * 60 * 60 * 1000)
    expect(getTimeLabelByDate(target)).toBe('一周内')
  })

  it('classifies a date within thirty days as "三十天内"', () => {
    const target = new Date(now - 10 * 24 * 60 * 60 * 1000)
    expect(getTimeLabelByDate(target)).toBe('三十天内')
  })

  it('classifies long ago as "更早"', () => {
    const target = new Date(now - 90 * 24 * 60 * 60 * 1000)
    expect(getTimeLabelByDate(target)).toBe('更早')
  })
})
