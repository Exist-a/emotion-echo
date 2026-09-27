/**
 * webllmEngine 接口 + Stub + Dynamic（Lane O · T2 → T3）。
 *
 * 目的：定义端侧推理引擎的**接口契约** + 占位 Stub 实现 + Dynamic 动态加载实现。
 *   - Stub：永远可跑，UI 不崩（mock fallback 路径）
 *   - Dynamic：异步尝试 `import('@mlc-ai/web-llm')`；失败抛清晰错误，T3 接真引擎时复用
 *
 * T2 末（当前）：Dynamic engine 完成 dynamic import 链路验证 + 状态对外暴露，但不
 * 实际触发 `CreateMLCEngine`（避免 ~1GB 模型权重下载）；T3 IAB 验证时把 chat() 接到
 * `engine.chat.completions.create({ stream: true })`。
 *
 * 接口稳定性：production bundle **不打包** @mlc-ai/web-llm —— Dynamic 仅用
 * `await import(...)` 异步加载，**禁止 static import 形式**。架构契约
 * 由 `__tests__/webllmEngine.dynamicImport.test.ts` 保证。
 */

/** 端侧推理消息（与 OpenAI 兼容，简化版 —— T3 接入时按 WebLLM 文档扩展） */
export interface ChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: string
}

/** init 输入 —— 模型 ID + 缓存选项（v0.2 §4.2 "缓存后端" + §4.3 "Web Worker"） */
export interface InitOptions {
  /** 模型 ID（如 "Qwen3-1.7B-q4f16_1-MLC"，来自 webllm config.ts 预置表） */
  modelId: string
  /** 缓存后端（Cache API / IndexedDB / OPFS —— v0.2 §4.2 评估待定） */
  cacheBackend?: 'cache' | 'indexeddb' | 'opfs'
  /** 是否启用 Web Worker 隔离（v0.2 §4.3 默认 true） */
  useWorker?: boolean
}

/** chat 输入 —— 消息历史 + 生成参数 */
export interface ChatOptions {
  messages: ChatMessage[]
  temperature?: number
  maxTokens?: number
  /** 流式回调 —— 每收到一个 delta 调一次 */
  onDelta?: (delta: string) => void
}

/**
 * 端侧推理引擎契约。
 *
 * 真实实现（T3 接入 @mlc-ai/web-llm）：
 *   - init: dynamic import('web-llm') → CreateMLCEngine 或 CreateWebWorkerMLCEngine
 *   - chat: engine.chat.completions.create({ stream: true }) → 流式回调
 *   - abort: engine.interrupt()
 *   - dispose: engine.unload() / engine.terminate()
 */
export interface WebLLMEngine {
  /** 模型 ID（init 后才设置） */
  readonly modelId: string
  /** 是否已 init 完成 */
  readonly loaded: boolean
  /** 初始化模型加载 + 缓存 + Worker（async —— 通常 1~3 秒 warm path） */
  init(options: InitOptions): Promise<void>
  /** 流式 chat —— 触发 onDelta 直到 done（async 返回 null on abort） */
  chat(options: ChatOptions): Promise<string | null>
  /** 中断当前 chat（v0.2 §5.4 "10 秒首 token → 切云端"） */
  abort(): void
  /** 卸载模型 + 释放 Worker（v0.2 §4.2 缓存保留，仅释放运行时） */
  dispose(): Promise<void>
}

/**
 * Stub 工厂：返回满足接口契约但不真请求 WebLLM 的占位实现。
 *
 * 用途：
 *   1. 单元测试：让上层组件能 mock 注入（DI 模式，AGENTS §3.1）
 *   2. SSR 安全：服务端渲染时拿到一个永远返回"MOCK" 的安全实现
 *   3. T3 前的开发：DBA 模型未接入时 UI 不崩
 *
 * chat() 返回 stub 字符串："[Stub Engine] 收到 N 条消息 —— T3 接入真引擎"
 */
export function createStubEngine(): WebLLMEngine {
  let loaded = false
  let modelId = ''
  return {
    get modelId() {
      return modelId
    },
    get loaded() {
      return loaded
    },
    async init(options: InitOptions): Promise<void> {
      // 模拟加载耗时（毫秒级 —— T3 真引擎是秒级，测试不需要真实延迟）
      await Promise.resolve()
      modelId = options.modelId
      loaded = true
    },
    async chat(options: ChatOptions): Promise<string | null> {
      if (!loaded) {
        throw new Error('Engine not loaded; call init() first')
      }
      // Stub 流式回调（虽然非真流 —— 但接口契约一致）
      if (options.onDelta) {
        const reply = `[Stub Engine] 收到 ${options.messages.length} 条消息（model=${modelId}）—— T3 接入真引擎`
        options.onDelta(reply)
      }
      return '[Stub Engine] 收到 N 条消息 —— T3 接入真引擎'
    },
    abort(): void {
      // Stub 无 inflight chat，no-op
    },
    async dispose(): Promise<void> {
      await Promise.resolve()
      loaded = false
      modelId = ''
    },
  }
}

/* ------------------------------------------------------------------ */
/*  Dynamic engine：T2 末新增 —— 异步 import + 状态对外暴露             */
/* ------------------------------------------------------------------ */

/** Dynamic engine 生命周期阶段（暴露给 UI 显示状态） */
export type DynamicEnginePhase =
  | 'idle' // 未触发 import
  | 'importing' // await import 进行中
  | 'loaded' // dynamic import 成功，模块可引用
  | 'unavailable' // dynamic import 失败（optional dep 未装 / 安装失败）

/** Dynamic engine 状态对象 —— 与 engine 实例同生命周期 */
export interface DynamicEngineState {
  phase: DynamicEnginePhase
  error: string | null
  /** dynamic import 加载的模块引用（loaded 时非空；类型断言由调用方处理） */
  module: unknown | null
}

/**
 * Dynamic 工厂：返回 { engine, state } 二元组。
 *
 * 行为：
 *   1. init(options) 首次触发 dynamic import('@mlc-ai/web-llm')；记录 phase + error
 *   2. import 失败抛清晰错误（指引装 optional dep）—— **不静默降级**（AGENTS §3.2 / §六）
 *   3. import 成功但 T2 末不真创建 MLCEngine（避免下载 ~1GB 权重）；chat() 抛
 *      `[Dynamic Engine] Real chat() not wired` 指引 T3 接入
 *
 * T3 接入点（chat() 内）：
 *   const webllm = state.module as typeof import('@mlc-ai/web-llm')
 *   const realEngine = options.useWorker !== false
 *     ? await webllm.CreateWebWorkerMLCEngine(engineRef, webllmConfig)
 *     : await webllm.CreateMLCEngine(engineRef, webllmConfig)
 *   realEngine.chat.completions.create({ messages, stream: true }) → 流式 onDelta
 *
 * 设计动机：T2 末只验证 dynamic import 链路 + 暴露状态；T3 才做完整端到端真实推理。
 */
export function createDynamicEngine(): { engine: WebLLMEngine; state: DynamicEngineState } {
  let loaded = false
  let modelId = ''
  // _aborted 标志 —— T2 末未消费（T3 接 chat() 流式时读；当前 chat 抛错不读此标志）
  let _aborted = false

  const state: DynamicEngineState = {
    phase: 'idle',
    error: null,
    module: null,
  }

  const engine: WebLLMEngine = {
    get modelId() {
      return modelId
    },
    get loaded() {
      return loaded
    },

    async init(options: InitOptions): Promise<void> {
      // 仅首次触发 dynamic import；后续 init 直接调 over
      if (state.phase === 'idle' || (state.phase === 'unavailable' && !state.module)) {
        state.phase = 'importing'
        state.error = null
        try {
          // 唯一引用方式：dynamic import（架构契约 —— 静态源禁止任何 `@mlc-ai/web-llm` 静态引用写法）
          const mod: unknown = await import('@mlc-ai/web-llm')
          state.module = mod
          state.phase = 'loaded'
        } catch (e) {
          state.phase = 'unavailable'
          const msg = e instanceof Error ? e.message : String(e)
          state.error = msg
          // 不静默降级：让上层知道是 optional dep 未装 / 安装失败 / 路径不可达
          throw new Error(
            `[Dynamic Engine] @mlc-ai/web-llm dynamic import failed: ${msg}. ` +
              'This is expected if optional dep @mlc-ai/web-llm is not installed ' +
              '(pnpm install 失败可选依赖不阻断主安装，但 runtime dynamic import 会抛). ' +
              'Run `pnpm install` (with optionalDependencies enabled) to enable real engine.',
          )
        }
      }

      // T2 末：dynamic import 链路已通，但不实际 CreateMLCEngine（避免 ~1GB 权重下载）
      // T3 接入：见本函数 JSDoc 第 2 段
      modelId = options.modelId
      loaded = true
      _aborted = false
    },

    async chat(_options: ChatOptions): Promise<string | null> {
      if (!loaded) {
        throw new Error('Engine not loaded; call init() first')
      }
      // T2 末：抛清晰错误而非静默 mock（让 T3 接入点明显）
      throw new Error(
        '[Dynamic Engine] Real chat() not wired in T2·Lane O. ' +
          'T3 will wire to webllm MLCEngine.chat.completions.create({ stream: true }). ' +
          `modelId=${modelId}, dynamic module phase=${state.phase}.`,
      )
    },

    abort(): void {
      _aborted = true
    },

    async dispose(): Promise<void> {
      // T3 接入：state.module?.unload?.() 或 Worker.terminate()
      _aborted = false
      loaded = false
      modelId = ''
      // state 保留 phase/module —— 下次 init 可重 import
    },
  }

  return { engine, state }
}