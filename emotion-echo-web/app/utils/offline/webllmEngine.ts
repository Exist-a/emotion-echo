/**
 * webllmEngine 接口 + Stub（Lane O · T2 · WebLLM Demo 契约骨架）。
 *
 * 目的：定义端侧推理引擎的**接口契约** + 占位 Stub 实现。**不** 真接 @mlc-ai/web-llm
 * （T3 才装依赖 + dynamic import；协议 §二 白名单）。
 *
 * 接口稳定性：production bundle 不打包本模块的实际运行时 —— T3 接入时通过 dynamic
 * import('@mlc-ai/web-llm') 在 init() 内异步加载。
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