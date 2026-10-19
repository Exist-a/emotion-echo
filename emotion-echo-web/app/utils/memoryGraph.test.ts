// app/utils/memoryGraph.test.ts
//
// CL-08a · A6：记忆图 → ECharts 选项（纯函数契约）。

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { buildMemoryGraphOption, hasMemoryGraph, nodeSize } from './memoryGraph'
import type { MemoryGraph } from '~/types/memory'

const sample: MemoryGraph = {
  nodes: [
    { id: 'user:1', label: '我', kind: 'user', weight: 2 },
    { id: 'entity:后端工程师', label: '后端工程师', kind: 'object', weight: 1 },
    { id: 'entity:跑步', label: '跑步', kind: 'object', weight: 1 },
  ],
  edges: [
    { source: 'user:1', target: 'entity:后端工程师', label: '职业' },
    { source: 'user:1', target: 'entity:跑步', label: '爱好' },
  ],
}

describe('buildMemoryGraphOption', () => {
  it('空图返回空 series（交给 BaseChart 显示空态）', () => {
    const opt = buildMemoryGraphOption({ nodes: [], edges: [] })
    expect(opt.series).toEqual([])
  })

  it('把节点与边映射成 ECharts graph 系列', () => {
    const opt = buildMemoryGraphOption(sample)
    const series = (opt.series as any[])[0]
    expect(series.type).toBe('graph')
    expect(series.layout).toBe('force')
    expect(series.data).toHaveLength(3)
    expect(series.links).toHaveLength(2)
    expect(series.links[0]).toMatchObject({
      source: 'user:1',
      target: 'entity:后端工程师',
      relation: '职业',
    })
  })

  it('中心节点使用用户色，实体节点尺寸随权重增长', () => {
    const opt = buildMemoryGraphOption(sample)
    const data = (opt.series as any[])[0].data
    const center = data.find((n: any) => n.id === 'user:1')
    expect(center.name).toBe('我')
    expect(center.symbolSize).toBe(nodeSize(2))
    expect(center.itemStyle.color).toBe('#8ebaa8')
  })

  it('节点 label 缺失时回落 id（不渲染空标签）', () => {
    const opt = buildMemoryGraphOption({
      nodes: [{ id: 'entity:x', label: '', kind: 'object', weight: 0 }],
      edges: [],
    })
    expect((opt.series as any[])[0].data[0].name).toBe('entity:x')
  })
})

describe('nodeSize', () => {
  it('基础尺寸 22，按权重增长并封顶', () => {
    expect(nodeSize(0)).toBe(22)
    expect(nodeSize(1)).toBe(27)
    expect(nodeSize(99)).toBe(22 + 6 * 5)
  })

  it('非法权重按 0 处理', () => {
    expect(nodeSize(Number.NaN)).toBe(22)
    expect(nodeSize(-3)).toBe(22)
  })
})

describe('hasMemoryGraph', () => {
  it('有节点才算有图', () => {
    expect(hasMemoryGraph(null)).toBe(false)
    expect(hasMemoryGraph({ nodes: [], edges: [] })).toBe(false)
    expect(hasMemoryGraph(sample)).toBe(true)
  })
})

describe('echarts 注册契约（nuxt-echarts 树摇）', () => {
  // CL-08a 生产构建实测抓出的**静默失败**：nuxt.config.ts 的 echarts.charts 只注册了
  // Bar/Line/Pie/Radar ⇒ graph 系列**不画任何东西**（容器在、canvas 无、控制台无报错）。
  // 这条门禁把"注册 GraphChart"从口头约定变成可红可绿的断言。
  it('nuxt.config.ts 必须注册 GraphChart', () => {
    const cfg = readFileSync(resolve(process.cwd(), 'nuxt.config.ts'), 'utf8')
    expect(cfg).toMatch(/charts:\s*\[[\s\S]*?"GraphChart"/)
  })
})
