// composables/useAIStream.ts
import { ref, onUnmounted } from "vue";
import { useRuntimeConfig } from "#app";

/**
 * AI 流式对话核心 Hook
 * 核心能力：发起流式请求、逐块解析回复、中断请求、组件卸载清理
 */
export function useAIStream() {
  // ========== 1. 核心响应式状态 ==========
  /** 流式拼接的回复内容（打字机效果核心） */
  const streamContent = ref("");
  /** 请求加载状态 */
  const loading = ref(false);
  /** 错误信息 */
  const error = ref("");

  // ========== 2. 取消控制器（用于中断流式请求） ==========
  let abortController: AbortController | null = null;

  // ========== 3. 核心方法：发起流式请求 ==========
  /**
   * 发起 AI 流式对话请求
   * @param prompt 用户输入的提问内容
   */
  const fetchAIStream = async (prompt: string) => {
    // 前置校验：仅在客户端执行（避免 SSR 报错）
    if (!import.meta.client) return;

    // 重置状态
    loading.value = true;
    error.value = "";
    streamContent.value = "";

    // 初始化取消控制器
    abortController = new AbortController();
    const runtimeConfig = useRuntimeConfig();

    try {
      // 调用 Nuxt 内置 $fetch（底层是原生 fetch，支持流式）
      const response = (await $fetch(
        runtimeConfig.public.AI_STREAM_API as string,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${import.meta.client ? localStorage.getItem("access_token") : ""}`,
          },
          body: {
            model: runtimeConfig.public.LLM_MODEL,
            messages: [{ role: "user", content: prompt }],
            stream: true,
          },
          signal: abortController.signal,
          // 👇 核心修改：显式返回原始 Response 对象（替代 false，更稳定）
         responseType:'stream'
        },
      )) as Response;

      // 校验响应状态
      if (!response.ok) {
        throw new Error(`请求失败：${response.status} ${response.statusText}`);
      }
      // 获取流式读取器
      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error("当前浏览器不支持流式响应");
      }

      // 初始化解码器（处理二进制流 → 字符串）
      const decoder = new TextDecoder("utf-8");
      // 逐块读取流式响应（核心逻辑）
      while (true) {
        const { done, value } = await reader.read();
        // 流读取完成则退出循环
        if (done) break;

        // 解析单块内容（适配 OpenAI 通用 SSE 格式）
        const chunk = decoder.decode(value, { stream: true });
        const newContent = parseLLMChunk(chunk);
        // 拼接内容（实现打字机效果）
        if (newContent) streamContent.value += newContent;
      }
    } catch (err: any) {
      // 排除手动取消的错误（用户主动中断）
      if (err.name !== "AbortError") {
        error.value = err.message || "流式请求失败";
      }
    } finally {
      loading.value = false;
    }
  };

  // ========== 4. 辅助方法：解析大模型分块 ==========
  /**
   * 解析单块流式数据（适配 OpenAI 规范，可按需扩展）
   * @param chunk 单块原始字符串
   * @returns 解析后的文本（无则返回空）
   */
  const parseLLMChunk = (chunk: string): string => {
    let content = "";
    // 按行分割（SSE 格式每行是一个数据块）
    const lines = chunk.split("\n").filter((line) => line.trim());
    lines.forEach((line) => {
      // 过滤 SSE 格式的 data: 前缀
      if (line.startsWith("data: ")) {
        const data = line.slice(6).trim();
        // 跳过结束标记
        if (data === "[DONE]") return;
        try {
          // 解析 JSON 并提取回复内容
          const parsed = JSON.parse(data);
          content += parsed.choices?.[0]?.delta?.content || "";
        } catch {}
      }
    });
    return content;
  };

  // ========== 5. 辅助方法：中断流式请求 ==========
  /** 中断当前流式请求（比如用户点击“取消”按钮） */
  const cancelStream = () => {
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    loading.value = false;
  };

  // ========== 6. 组件卸载清理（避免内存泄漏） ==========
  onUnmounted(() => {
    cancelStream();
  });

  // ========== 7. 暴露给组件的状态和方法 ==========
  return {
    streamContent, // 流式回复内容
    loading, // 加载状态
    error, // 错误信息
    fetchAIStream, // 发起流式请求
    cancelStream, // 中断请求
  };
}
