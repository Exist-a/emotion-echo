/**
 * routeDecision 路由决策（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：v0.2 §5.1 路由决策树纯函数。**不** 依赖模型/网络/UI —— 可在 test 中以纯输入断言。
 *
 * 决策优先级（v0.2 §5.1 修正）：
 *   1. 设备不支持 → cloud
 *   2. 端侧模型未加载完成 → cloud
 *   3. 命中一级高危 → 联网走 cloud，离线走 fallback_hotline（兜住"高危×断网"）
 *   4. 输入文本 >500 字 → cloud
 *   5. 离线状态 → local（前提：上述条件不命中）
 *   6. 默认 → local
 *
 * "高危×断网"分支特别说明：不依赖模型判断、不依赖网络 —— 直接走固定模板填空，
 * 这是 v0.2 §5.1 强约束护栏（与 golden set metrics.HOTLINE 一致）。
 */

export type DeviceCap = 'webgpu' | 'software' | 'unsupported'
export type RouteLayer = 'daily' | 'high_risk' | 'long_input' | 'personality' | 'emotion'

/** 一级高危 token（v0.2 §6.3 + §5.1）—— 命中即转危机路径 */
export const HIGH_RISK_TOKENS: readonly string[] = ['自杀', '自残', '不想活', '结束生命', '想死', '不想 活']

/** 危机回应固定模板（v0.2 §5.1 离线兜底 + §6.1 高危层必含热线） */
export const HIGH_RISK_HOTLINE_TEMPLATE =
  '我听到你说的话了，你的感受很重要。请拨打心理援助热线 400-161-9995，' +
  '24 小时有人倾听。你也可以告诉我更多。'

/** v0.2 §5.1 超长输入阈值（字） */
export const LONG_INPUT_THRESHOLD = 500

export interface RouteContext {
  /** 设备 WebGPU 检测结果（来自 detectWebGPU） */
  deviceCap: DeviceCap
  /** 端侧模型是否已加载完成（来自 webllmEngine.init） */
  modelLoaded: boolean
  /** 是否联网（来自 navigator.onLine 或心跳） */
  online: boolean
  /** 用户输入文本（用于长度 + 高危 token 扫描） */
  input: string
  /** 当前对话层级（与 scripts/on-device-golden/golden_set.jsonl 五分层一致） */
  layer: RouteLayer
}

export type Route = 'local' | 'cloud' | 'fallback_hotline'

export interface RouteDecision {
  route: Route
  /** 决策原因 —— 用于 UI 显示 "为什么走这条路"（v0.2 §5.4 来源告知） */
  reason: string
  /** 仅 fallback_hotline 时携带固定模板（golden set 高危层护栏） */
  template?: string
}

/** 检测输入文本是否命中一级高危 token */
function isHighRisk(input: string): boolean {
  for (const token of HIGH_RISK_TOKENS) {
    if (input.includes(token)) return true
  }
  return false
}

/** 检测输入是否超长（按字符数，匹配中文用例） */
function isTooLong(input: string): boolean {
  return input.length > LONG_INPUT_THRESHOLD
}

/**
 * 决策主函数 —— 严格按 v0.2 §5.1 优先级顺序匹配。
 *
 * **安全护栏原则**：高危**优先于**设备/模型/超长 —— 危机响应永远第一优先级。
 * 即使设备不支持或模型未加载，命中高危关键词的输入也必须被识别并转云端/兜底模板，
 * 不能因端侧条件不满足而被静默忽略（v0.2 §5.1 "高危×离线"分支的精神推广）。
 *
 * 优先级顺序：
 *   1. 高危关键词（联网 → cloud；离线 → fallback_hotline 兜底模板）
 *   2. 设备不支持 WebGPU（含 software）→ cloud
 *   3. 端侧模型未加载完成 → cloud
 *   4. 输入超长（>500 字） → cloud
 *   5. 离线状态 → local
 *   6. 默认 → local
 */
export function decideRoute(ctx: RouteContext): RouteDecision {
  // 优先级 1（安全护栏）：高危关键词 —— 设备不支持也走 cloud 转专业；
  // 离线时走固定模板填空兜底，不依赖模型/网络
  if (isHighRisk(ctx.input)) {
    if (!ctx.online) {
      return {
        route: 'fallback_hotline',
        reason: '命中高危关键词 + 离线 → 模板填空兜底',
        template: HIGH_RISK_HOTLINE_TEMPLATE,
      }
    }
    return { route: 'cloud', reason: '命中高危关键词 → 云端专业处理' }
  }
  // 优先级 2：设备不支持（含 software 暂不区分 → 走 cloud 兜底）
  if (ctx.deviceCap !== 'webgpu') {
    return { route: 'cloud', reason: `设备不支持端侧推理（${ctx.deviceCap}）` }
  }
  // 优先级 3：模型未加载
  if (!ctx.modelLoaded) {
    return { route: 'cloud', reason: '端侧模型未加载完成' }
  }
  // 优先级 4：超长输入
  if (isTooLong(ctx.input)) {
    return { route: 'cloud', reason: `输入超长（>${LONG_INPUT_THRESHOLD}字）` }
  }
  // 优先级 5：离线状态（前提：上述条件不命中）
  if (!ctx.online) {
    return { route: 'local', reason: '离线状态 → 端侧兜底' }
  }
  // 优先级 6：默认 → 端侧
  return { route: 'local', reason: '默认 → 端侧推理' }
}