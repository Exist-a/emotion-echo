// app/types/memory.ts
//
// CL-08a · A6：「我的记忆」页的数据契约（与 BFF `/api/v1/memory/*` 对齐）。

/** 记忆图节点（kind: user / subject / object）。 */
export interface MemoryGraphNode {
  id: string
  label: string
  kind: string
  /** 关联事实数（前端用来调节点大小） */
  weight: number
}

/** 记忆图边（label = predicate）。 */
export interface MemoryGraphEdge {
  source: string
  target: string
  label: string
}

export interface MemoryGraph {
  nodes: MemoryGraphNode[]
  edges: MemoryGraphEdge[]
}

/** 自述条写入结果（回执固定文案，不含抽取结果 —— 抽取是异步的）。 */
export interface MemoryNoteResult {
  noteId: number
  ackText: string
}

// CL-08d：管理页列举条目（GET /memory/items）。
export interface MemoryItem {
  kind: 'fact' | 'note'
  id: number
  text: string
  sourceKind?: string
  sourceId?: number
  createdAt?: string
}
