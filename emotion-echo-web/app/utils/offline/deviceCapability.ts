/**
 * deviceCapability 设备能力检测（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：v0.2 §4.2 设备能力检测的纯函数层 —— **不** 真连 WebGPU 适配器（v0.3 §F.2 #2 +
 * memory `tdd-contract-vs-behavioral-tests.md` —— happy-dom mock 失真）。
 *
 * 输入：注入 navigator（便于 vitest mock）；输出 3 态字符串（async）。
 * 真机调用：UI 入口 `app/pages/demo/local-llm.vue` 在 onMounted 异步调用此函数。
 *
 * 为什么 async：navigator.gpu.requestAdapter() 返回 Promise<GPUAdapter | null>，
 * 同步函数无法等待 Promise 解析 —— 真机场景必须 await 才能区分"adapter 解析 null
 * （WebGPU 不可用）"和"adapter 解析成功"。测试 mock 也以 Promise 形式返回。
 */

export type DeviceClass = 'webgpu' | 'software' | 'unsupported'

/**
 * 探测 WebGPU 可用性 —— 异步检测返回 3 态字符串。
 *
 * @param navigatorLike 通常是 globalThis.navigator；测试可注入 mock。
 * @returns 'webgpu' 硬件加速可用 / 'software' 软件回退可用（保留扩展位，当前 v0.2
 *          不区分 software，统一返回 unsupported）/ 'unsupported' 无 WebGPU 能力
 */
export async function detectWebGPU(navigatorLike: unknown): Promise<DeviceClass> {
  const gpu = (navigatorLike as { gpu?: { requestAdapter?: () => Promise<unknown> } } | null | undefined)?.gpu
  if (!gpu || typeof gpu.requestAdapter !== 'function') {
    return 'unsupported'
  }
  try {
    const adapter = await gpu.requestAdapter()
    if (adapter) {
      return 'webgpu'
    }
    return 'unsupported'
  } catch {
    return 'unsupported'
  }
}