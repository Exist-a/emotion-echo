/**
 * deviceCapability 字面量契约测试（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：detectWebGPU 是纯函数，注入 navigator mock 后断言返回值。**不** 真连 WebGPU 适配器
 * （v0.3 §F.2 #2 + memory `tdd-contract-vs-behavioral-tests.md` —— happy-dom 失真）。
 *
 * 来源 v0.2 §4.2（设备能力检测）+ §三 设备分级（桌面独显/集显/移动）。
 */
import { describe, expect, it } from 'vitest'
import { detectWebGPU, type DeviceClass } from '../deviceCapability'

/**
 * 测试夹具：构造 minimal "navigator" 形状（含或不包含 gpu）。
 * 类型故意取 unknown 接口而非 globalThis.Navigator，避免 happy-dom 全局类型耦合。
 */
function makeNavigator(opts: {
  hasGpu?: boolean
  hasAdapter?: boolean
  gpuRequestAdapterThrows?: boolean
}): unknown {
  if (opts.hasGpu === false) {
    return { /* 无 gpu */ }
  }
  const gpu = {
    requestAdapter: opts.gpuRequestAdapterThrows
      ? () => {
          throw new Error('mock GPU init failure')
        }
      : () =>
          Promise.resolve(
            opts.hasAdapter === false ? null : { name: 'mock-adapter' }
          ),
  }
  return { gpu }
}

describe('detectWebGPU', () => {
  it('当 navigator 没有 gpu 属性时，返回 "unsupported"', async () => {
    expect(
      await detectWebGPU(makeNavigator({ hasGpu: false }) as never)
    ).toBe('unsupported')
  })

  it('当 navigator.gpu.requestAdapter 抛错时，返回 "unsupported"', async () => {
    expect(
      await detectWebGPU(
        makeNavigator({ hasGpu: true, gpuRequestAdapterThrows: true }) as never
      )
    ).toBe('unsupported')
  })

  it('当 navigator.gpu.requestAdapter 解析 null 时，返回 "unsupported"', async () => {
    expect(
      await detectWebGPU(makeNavigator({ hasGpu: true, hasAdapter: false }) as never)
    ).toBe('unsupported')
  })

  it('当 navigator.gpu.requestAdapter 解析成功时，返回 "webgpu"', async () => {
    expect(
      await detectWebGPU(makeNavigator({ hasGpu: true, hasAdapter: true }) as never)
    ).toBe('webgpu')
  })

  it('返回类型必须是 DeviceClass 联合体的子集', async () => {
    const allowed: DeviceClass[] = ['webgpu', 'software', 'unsupported']
    const results = await Promise.all([
      detectWebGPU(makeNavigator({ hasGpu: false }) as never),
      detectWebGPU(makeNavigator({ hasGpu: true, hasAdapter: true }) as never),
      detectWebGPU(
        makeNavigator({ hasGpu: true, gpuRequestAdapterThrows: true }) as never
      ),
    ])
    for (const r of results) {
      expect(allowed).toContain(r)
    }
  })
})