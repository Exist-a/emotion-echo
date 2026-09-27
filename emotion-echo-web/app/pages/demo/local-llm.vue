<script setup lang="ts">
/**
 * local-llm.vue — WebLLM 端侧推理 Demo（Lane O · T2 → T3）。
 *
 * T2 末（当前）：渲染契约落地证明 + Dynamic engine 异步探测。
 *   - 设备能力 + 路由决策：契约骨架（T2#1）
 *   - Stub engine：永远可跑，UI 不崩（T2#1）
 *   - Dynamic engine：异步 `import('@mlc-ai/web-llm')`，显示 phase 给用户；**不**真
 *     创建 MLCEngine（避免 ~1GB 权重下载），T3 IAB 验证时 chat() 接到真引擎
 *
 * 设计约束（v0.3 §C.1 + §C.6 + 协议 §二/§八）：
 *   - 独立路由 /demo/local-llm（生产 build 排除，§C.6 阶段一）
 *   - 不依赖 useAIStreamHandler（协议 §二 stage1 禁触）
 *   - 不依赖 useUserStore / useApi（避开 auth.global.ts 共享列握手）
 *   - 不静态 import @mlc-ai/web-llm（dynamic import 隔离；架构测试保证）
 *   - <ClientOnly> 包住所有 WebGPU 调用点（decision-24 SSR 隔离）
 *   - layout: 'default'（不用 nav layout）
 *   - ssr: false（WebGPU/Worker 客户端专用）
 */
import { detectWebGPU, type DeviceClass } from '~/utils/offline/deviceCapability'
import {
  decideRoute,
  type RouteContext,
  type RouteDecision,
} from '~/utils/offline/routeDecision'
import {
  createStubEngine,
  createDynamicEngine,
  type DynamicEnginePhase,
} from '~/utils/offline/webllmEngine'

definePageMeta({
  layout: 'default',
  ssr: false,
})

// 三个 ref —— 仅在 <ClientOnly> 子树内有效
const deviceCap = ref<DeviceClass>('unsupported')
const sampleInput = ref('今天上班有点累')
const routeDecision = ref<RouteDecision | null>(null)
const engine = ref<ReturnType<typeof createStubEngine> | null>(null)
const initStatus = ref<string>('not started')

// Dynamic engine 状态（T2 末新增）
const dynamicEngine = ref<ReturnType<typeof createDynamicEngine> | null>(null)
const dynamicPhase = ref<DynamicEnginePhase>('idle')
const dynamicError = ref<string | null>(null)
const dynamicTriggering = ref(false)

// 客户端挂载后才执行 WebGPU 检测 + stub 引擎初始化
onMounted(async () => {
  deviceCap.value = await detectWebGPU(navigator)
  const ctx: RouteContext = {
    deviceCap: deviceCap.value,
    modelLoaded: true, // stub 引擎已加载则视为已加载
    online: typeof navigator !== 'undefined' ? navigator.onLine : true,
    input: sampleInput.value,
    layer: 'daily',
  }
  routeDecision.value = decideRoute(ctx)
  // stub 引擎初始化（永远 fallback 安全）
  engine.value = createStubEngine()
  await engine.value.init({ modelId: 'Qwen3-1.7B-q4f16_1-MLC' })
  initStatus.value = engine.value.loaded ? 'stub ready' : 'init failed'

  // Dynamic engine 工厂创建（不触发 import —— 等用户点按钮才触发）
  dynamicEngine.value = createDynamicEngine()
})

/** 用户点按钮触发 dynamic engine init —— 验证 dynamic import 链路 */
const triggerDynamicInit = async () => {
  if (!dynamicEngine.value || dynamicTriggering.value) return
  dynamicTriggering.value = true
  dynamicError.value = null
  try {
    await dynamicEngine.value.engine.init({ modelId: 'Qwen3-1.7B-q4f16_1-MLC' })
  } catch (e) {
    // 抛错落到 state.error；同时 ref 同步
    dynamicError.value = e instanceof Error ? e.message : String(e)
  } finally {
    dynamicPhase.value = dynamicEngine.value.state.phase
    dynamicTriggering.value = false
  }
}

const reEvaluate = async () => {
  if (typeof navigator === 'undefined') return
  const ctx: RouteContext = {
    deviceCap: await detectWebGPU(navigator),
    modelLoaded: engine.value?.loaded ?? false,
    online: navigator.onLine,
    input: sampleInput.value,
    layer: 'daily',
  }
  routeDecision.value = decideRoute(ctx)
}
</script>

<template>
  <div class="local-llm-demo">
    <header class="demo-header">
      <h1>WebLLM 端侧推理 Demo</h1>
      <p class="wip-banner">
        WIP — 当前为契约骨架（T2 · Lane O）· T3 才接真 @mlc-ai/web-llm 引擎
      </p>
    </header>

    <ClientOnly>
      <section class="demo-content">
        <div class="panel">
          <h2>设备能力</h2>
          <p data-testid="device-cap">detectWebGPU → <code>{{ deviceCap }}</code></p>
        </div>

        <div class="panel">
          <h2>路由决策（输入模拟）</h2>
          <input
            v-model="sampleInput"
            class="demo-input"
            type="text"
            placeholder="输入测试文本"
            @input="reEvaluate"
          >
          <p v-if="routeDecision" data-testid="route-decision">
            route = <code>{{ routeDecision.route }}</code> /
            reason = <code>{{ routeDecision.reason }}</code>
          </p>
        </div>

        <div class="panel">
          <h2>引擎状态（stub）</h2>
          <p data-testid="engine-status">init → <code>{{ initStatus }}</code></p>
          <p class="stub-note">
            Stub 引擎返回固定占位字符串 —— T3 才接真实端侧推理。
          </p>
        </div>

        <div class="panel">
          <h2>Dynamic engine（真引擎探测）</h2>
          <p class="stub-note">
            异步 <code>import('@mlc-ai/web-llm')</code> 验证模块可达；T2 末不真创建 MLCEngine
            （避免 ~1GB 权重下载）。失败 = optional dep 未装，pnpm install 重试。
          </p>
          <p>
            phase = <code data-testid="dynamic-phase">{{ dynamicPhase }}</code>
          </p>
          <button
            class="demo-button"
            :disabled="dynamicTriggering"
            data-testid="dynamic-trigger"
            @click="triggerDynamicInit"
          >
            {{ dynamicTriggering ? '加载中…' : '尝试加载 @mlc-ai/web-llm' }}
          </button>
          <p v-if="dynamicError" class="dynamic-error" data-testid="dynamic-error">
            {{ dynamicError }}
          </p>
        </div>
      </section>

      <template #fallback>
        <p>SSR/服务端渲染中 —— 端侧能力检测需浏览器环境</p>
      </template>
    </ClientOnly>
  </div>
</template>

<style scoped>
.local-llm-demo {
  padding: 24px;
  max-width: 800px;
  margin: 0 auto;
}
.demo-header h1 {
  margin: 0 0 8px;
}
.wip-banner {
  background: #fff7e6;
  border-left: 4px solid #ffa940;
  padding: 8px 12px;
  margin: 0 0 24px;
  font-size: 14px;
  color: #874d00;
}
.demo-content {
  display: grid;
  gap: 16px;
}
.panel {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  background: #fafafa;
}
.panel h2 {
  margin: 0 0 12px;
  font-size: 16px;
}
.demo-input {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #ccc;
  border-radius: 4px;
  font-size: 14px;
  box-sizing: border-box;
  margin-bottom: 8px;
}
.stub-note {
  font-size: 12px;
  color: #999;
  margin: 8px 0 0;
}
.demo-button {
  padding: 8px 16px;
  font-size: 14px;
  border: 1px solid #409eff;
  background: #409eff;
  color: #fff;
  border-radius: 4px;
  cursor: pointer;
  margin-top: 8px;
}
.demo-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.dynamic-error {
  margin-top: 8px;
  padding: 8px;
  background: #fef0f0;
  border-left: 3px solid #f56c6c;
  font-size: 12px;
  color: #c45656;
  word-break: break-word;
}
</style>