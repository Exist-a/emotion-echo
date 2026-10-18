// app/utils/memoryGraph.ts
//
// CL-08a · A6：记忆图 → ECharts graph 选项（纯函数，便于单测）。
//
// 形态（用户裁定）：**中心辐射图** —— 「我」在中心，实体向外发散；
// 节点大小随关联事实数（weight）增长，边标签 = predicate。

import type { EChartsOption } from 'echarts'
import type { MemoryGraph, MemoryGraphNode } from '~/types/memory'

/** 颜色按 kind 区分：我是主色，实体两种辅助色。 */
const KIND_COLOR: Record<string, string> = {
  user: '#8ebaa8',
  subject: '#c9a27a',
  object: '#8fa8c8',
}

/** 节点半径：基础 22px，每多一条关联事实 +5px，上限 +30px。 */
export function nodeSize(weight: number): number {
  const w = Number.isFinite(weight) && weight > 0 ? weight : 0
  return 22 + Math.min(w, 6) * 5
}

function toSeriesNode(n: MemoryGraphNode) {
  return {
    id: n.id,
    name: n.label || n.id,
    value: n.weight,
    symbolSize: nodeSize(n.weight),
    itemStyle: { color: KIND_COLOR[n.kind] || KIND_COLOR.object },
    label: { show: true, fontSize: 12 },
  }
}

/**
 * buildMemoryGraphOption 生成 ECharts 选项。
 *
 * 空图返回 `{ series: [] }` —— BaseChart 的 hasData 会因此显示「暂无数据」空态。
 */
export function buildMemoryGraphOption(graph: MemoryGraph): EChartsOption {
  if (!graph || !graph.nodes || graph.nodes.length === 0) {
    return { series: [] }
  }
  return {
    tooltip: {},
    series: [
      {
        type: 'graph',
        layout: 'force',
        roam: true,
        draggable: true,
        focusNodeAdjacency: false,
        force: { repulsion: 320, edgeLength: [90, 160], gravity: 0.08 },
        label: { show: true },
        edgeLabel: {
          show: true,
          fontSize: 11,
          // ECharts 的边标签不是字符串字段：把 predicate 放进 relation，再在 formatter 里读
          formatter: (p: any) => p?.data?.relation ?? '',
        },
        lineStyle: { color: '#c9d3cf', width: 1.2, curveness: 0.06 },
        emphasis: { focus: 'adjacency' },
        data: graph.nodes.map(toSeriesNode),
        links: (graph.edges || []).map(
          (e) =>
            ({
              source: e.source,
              target: e.target,
              relation: e.label,
            }) as any,
        ),
      },
    ],
  }
}

/** hasMemoryGraph 是否至少有可画的节点。 */
export function hasMemoryGraph(graph: MemoryGraph | null | undefined): boolean {
  return !!graph && Array.isArray(graph.nodes) && graph.nodes.length > 0
}
