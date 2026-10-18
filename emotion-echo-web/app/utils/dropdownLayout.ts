// app/utils/dropdownLayout.ts
//
// CL-06 · UI 轮：浮层（自绘下拉）的定位算术 —— 纯函数，不碰 DOM。
//
// 抽出来的原因：这段"往下放不下就翻到上方"的判断是浮层最典型的静默损坏点
// （面板贴住窗口底边、被裁掉一半、翻上去顶出屏幕），而它只是几个不等式 —— 适合单测。
// 真实像素结果仍由浏览器复验，但边界条件在这里先钉死。

export interface DropdownAnchor {
  /** 锚点（触发器）在视口坐标里的上边缘 */
  top: number
  /** 锚点下边缘 */
  bottom: number
}

export interface DropdownLayout {
  /** true = 向上展开（贴在锚点上方） */
  flipUp: boolean
  /** 内联 style 用；flipUp 时为 'auto' */
  top: number | 'auto'
  /** 内联 style 用；flipUp 时为到视口底部的距离，否则 'auto' */
  bottom: number | 'auto'
  maxHeight: number
}

const DEFAULT_MAX_HEIGHT = 264
const DEFAULT_MIN_HEIGHT = 132
const DEFAULT_GAP = 6

/**
 * 决定浮层往下还是往上、以及多高。
 *
 * 规则（刻意简单，便于解释与预测）：
 *   ① 下方能放下上限高度 ⇒ 向下，用满上限；
 *   ② 否则若上方比下方宽裕 ⇒ 向上；
 *   ③ 否则仍向下（翻了只会更糟）；
 *   ④ 高度 = min(上限, 该方向的可用空间)，但不低于最小可读高度 ——
 *      窗口极窄时宁可让面板自己内部滚动，也不给一个 20px 的条子。
 */
export function computeDropdownLayout(
  anchor: DropdownAnchor,
  viewportHeight: number,
  options: { maxHeight?: number; minHeight?: number; gap?: number } = {},
): DropdownLayout {
  const maxHeight = options.maxHeight ?? DEFAULT_MAX_HEIGHT
  const minHeight = options.minHeight ?? DEFAULT_MIN_HEIGHT
  const gap = options.gap ?? DEFAULT_GAP

  const below = viewportHeight - anchor.bottom - gap
  const above = anchor.top - gap
  const flipUp = below < maxHeight && above > below

  const available = flipUp ? above : below
  const height = Math.max(minHeight, Math.min(maxHeight, available))

  return {
    flipUp,
    top: flipUp ? 'auto' : anchor.bottom + gap,
    bottom: flipUp ? Math.max(0, viewportHeight - anchor.top + gap) : 'auto',
    maxHeight: height,
  }
}
