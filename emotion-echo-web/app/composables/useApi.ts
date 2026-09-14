// composables/useApi.ts - API 请求封装
import type { ApiResponse } from "~/types/api";
import { ApiError } from "~/types/api";
import { useUserStore } from "~/stores/user";
import { navigateTo } from "#app";
import { API_ROUTES } from "~/lib/apiRoutes";
import { getApiBaseUrl } from "../lib/apiBaseUrl";

export function useApi() {
  return {
    get,
    post,
    put,
    delete: del,
    request,
    streamRequest,
    getBaseUrl
  }
}

/**
 * API 请求封装
 * 处理：基础URL、Token自动附加、401自动刷新、429限流重试、错误统一处理
 */

// 基础配置（从 runtimeConfig 读取，支持运行时获取）
// PR-A: 改用 fail-fast helper（决策 18 #24）；不再静默回退到 8080
function getBaseUrl(): string {
  return getApiBaseUrl(useRuntimeConfig());
}

// 429 限流配置
const MAX_RETRY_COUNT = 2; // 减少重试次数
const DEFAULT_RETRY_DELAY_MS = 2000; // 延长退避基准

// Token 刷新锁，防止并发刷新
let refreshPromise: Promise<string | null> | null = null;

// 请求去重：记录正在进行的请求
// P2-R2-2: 加上 size 上限 + 失败时清理，避免 Map 无限增长导致内存泄漏
const pendingRequests = new Map<string, Promise<any>>();
const MAX_PENDING_REQUESTS = 100;  // 单次页面允许并发去重 GET 数

// 生成请求唯一标识
// P2-R2-1: JSON.stringify 顺序敏感问题——同语义请求若 key 顺序不同会被视为不同
// 修复：对 GET URL 的 query 参数按 key 排序后再生成 key
function getRequestKey(url: string, options: RequestInit = {}): string {
  const method = options.method || "GET";
  let body = "";
  if (options.body) {
    if (typeof options.body === "string") {
      body = options.body;
    } else {
      try {
        body = JSON.stringify(sortObjectKeys(options.body));
      } catch {
        body = String(options.body);
      }
    }
  }
  // GET 请求：URL 的 query 也按 key 排序
  const sortedUrl = method === "GET" ? sortUrlQuery(url) : url;
  return `${method}:${sortedUrl}:${body}`;
}

/** 递归对象 key 排序（深拷贝但 key 顺序确定） */
function sortObjectKeys(obj: any): any {
  if (Array.isArray(obj)) return obj.map(sortObjectKeys);
  if (obj && typeof obj === "object" && obj.constructor === Object) {
    const sorted: Record<string, any> = {};
    for (const k of Object.keys(obj).sort()) sorted[k] = sortObjectKeys(obj[k]);
    return sorted;
  }
  return obj;
}

/** URL query 参数按 key 排序（仅 GET 走此路径） */
function sortUrlQuery(url: string): string {
  const qIdx = url.indexOf("?");
  if (qIdx < 0) return url;
  const base = url.slice(0, qIdx);
  const query = url.slice(qIdx + 1);
  const params = query.split("&").filter(Boolean).sort();
  return params.length > 0 ? `${base}?${params.join("&")}` : base;
}

/**
 * P2-R2-2: pendingRequests 写入前检查 size 上限，避免 Map 无限增长
 */
function trackPending(key: string, promise: Promise<any>): void {
  if (pendingRequests.size >= MAX_PENDING_REQUESTS) {
    // LRU 简化版：删第一个 entry（FIFO）。最旧请求大概率已 settled 但未清理
    const firstKey = pendingRequests.keys().next().value;
    if (firstKey) pendingRequests.delete(firstKey);
  }
  pendingRequests.set(key, promise);
}

/**
 * P2-R2-3: 合并两个 AbortSignal——任一触发即中止
 */
function mergeSignals(a: AbortSignal, b: AbortSignal): AbortSignal {
  const ctrl = new AbortController();
  const onAbort = () => ctrl.abort();
  a.addEventListener("abort", onAbort);
  b.addEventListener("abort", onAbort);
  if (a.aborted || b.aborted) ctrl.abort();
  return ctrl.signal;
}

/**
 * 获取 AccessToken
 * P0-R2-1: SSR 从 cookie 读取；CSR 优先从 userStore 读取，fallback 到 cookie
 * （页面刷新后 Pinia 状态丢失，但 cookie 仍在）
 */
function getAccessToken(): string | null {
  if (import.meta.client) {
    const userStore = useUserStore();
    const storeToken = userStore.getAccessToken;
    if (storeToken) return storeToken;
    // fallback: 页面刷新后 Pinia 状态丢失，从 cookie 恢复
    const tokenCookie = useCookie("access_token");
    return tokenCookie.value || null;
  }
  // SSR 时从 cookie 读取
  const tokenCookie = useCookie("access_token");
  return tokenCookie.value || null;
}

/**
 * 设置 AccessToken（通过 userStore）
 * P0-R2-1: rememberMe 参数保留兼容性，不再影响存储策略
 */
function setAccessToken(token: string, expiresIn: number = 900, rememberMe?: boolean): void {
  const userStore = useUserStore();
  userStore.setAccessToken(token, expiresIn, rememberMe ?? false);
}

/**
 * 从 JWT Token 中解析 jti（唯一标识）
 * 用于刷新时回传给后端做 Token 轮换校验
 */
function getTokenJti(token: string | null): string | null {
  if (!token) return null;
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    // Base64Url -> Base64 转换
    const base64 = (parts[1] || "").replace(/-/g, "+").replace(/_/g, "/");
    const payload = JSON.parse(atob(base64));
    return payload.jti || null;
  } catch {
    return null;
  }
}

/**
 * 刷新 Token（带锁，防止并发）
 * 后端要求回传当前 AccessToken 的 jti，用于黑名单/轮换校验
 */
async function refreshToken(): Promise<string | null> {
  // 如果已有刷新在进行中，等待其结果
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = (async () => {
    try {
      const currentToken = getAccessToken();
      const jti = getTokenJti(currentToken);

      const body: Record<string, any> = {};
      if (jti) {
        body.jti = jti;
      }

      const res = await fetch(`${getBaseUrl()}${API_ROUTES.authRefresh.path}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
        credentials: "include", // 携带 HttpOnly Cookie
      });

      if (!res.ok) {
        throw new Error("刷新 Token 失败");
      }

      const data: ApiResponse<{ accessToken: string; expiresIn: number; rememberMe?: boolean }> = await res.json();

      if (data.code === 0) {
        setAccessToken(data.data.accessToken, data.data.expiresIn, data.data.rememberMe);
        return data.data.accessToken;
      }

      throw new Error(data.message);
    } catch (error) {
      console.error("刷新 Token 失败:", error);
      return null;
    } finally {
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

/**
 * 计算 429 限流后的等待时间（毫秒）
 * 优先使用后端 Retry-After，否则指数退避
 */
function getRetryDelayMs(response: Response, attempt: number): number {
  const retryAfter = response.headers.get("Retry-After");
  if (retryAfter) {
    const seconds = parseInt(retryAfter, 10);
    if (!isNaN(seconds)) {
      return seconds * 1000;
    }
  }
  // 指数退避：1s, 2s, 4s
  return Math.pow(2, attempt) * DEFAULT_RETRY_DELAY_MS;
}

/**
 * 清除登录状态
 */
function clearAuth(): void {
  const userStore = useUserStore();
  userStore.clearToken();
}

/**
 * 通用请求函数
 * 支持：401自动刷新、429限流自动重试（指数退避）、请求去重
 */
export async function request<T = any>(
  url: string,
  options: RequestInit = {},
  retryAttempt: number = 0
): Promise<T> {
  const fullUrl = url.startsWith("http") ? url : `${getBaseUrl()}${url}`;

  // 对于 GET 请求，添加去重逻辑
  const method = options.method || "GET";
  const requestKey = getRequestKey(url, options);
  
  if (method === "GET" && pendingRequests.has(requestKey)) {
    console.log(`[API] 请求去重，复用已有请求: ${method} ${url}`);
    return pendingRequests.get(requestKey)!;
  }

  // 设置默认 headers
  const isFormData = options.body instanceof FormData;
  const headers: Record<string, string> = {
    ...(!isFormData ? { "Content-Type": "application/json" } : {}),
    ...((options.headers as Record<string, string>) || {}),
  };

  // 附加 Token
  const token = getAccessToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  // 执行请求
  const requestPromise = (async () => {
    try {
      // P2-R2-3: 消费 NUXT_PUBLIC_REQUEST_TIMEOUT 配置（默认 30s）
      // 实现方式：合并已有 signal 与 timeout signal，任一触发即 abort
      const timeoutMs = Number(
        (typeof useRuntimeConfig === "function"
          ? useRuntimeConfig()?.public?.REQUEST_TIMEOUT
          : null) || 30000
      );
      const timeoutController = new AbortController();
      const timeoutId = setTimeout(() => timeoutController.abort(), timeoutMs);
      const existingSignal = options.signal;
      const combinedSignal = existingSignal
        ? mergeSignals(existingSignal, timeoutController.signal)
        : timeoutController.signal;
      let res = await fetch(fullUrl, {
        ...options,
        headers,
        credentials: "include", // 携带 Cookie
        signal: combinedSignal,
      });
      clearTimeout(timeoutId);

  // 处理 429 - 限流（优先于 401 处理）
  if (res.status === 429 && retryAttempt < MAX_RETRY_COUNT) {
    const delayMs = getRetryDelayMs(res, retryAttempt);
    console.warn(`[API] 触发限流(429)，等待 ${delayMs}ms 后重试(${retryAttempt + 1}/${MAX_RETRY_COUNT})`);
    await new Promise((resolve) => setTimeout(resolve, delayMs));
    return request(url, options, retryAttempt + 1);
  }

  // 处理 401 - Token 过期
  if (res.status === 401) {
    let errorData: ApiResponse;
    try {
      errorData = await res.json();
    } catch {
      errorData = { code: 10003, message: "Token 无效", data: null };
    }

    if (errorData.code === 10002) {
      // Token 过期，尝试刷新
      const newToken = await refreshToken();

      if (newToken) {
        // 重试原请求（保持 retryAttempt，401 刷新不计入 429 重试次数）
        headers["Authorization"] = `Bearer ${newToken}`;
        res = await fetch(fullUrl, {
          ...options,
          headers,
          credentials: "include",
        });
      } else {
        // 刷新失败，跳转登录
        clearAuth();
        if (import.meta.client) {
          navigateTo("/login", { replace: true });
        }
        throw new Error("登录已过期，请重新登录");
      }
    } else {
      // 其他 401 错误
      clearAuth();
      if (import.meta.client) {
        navigateTo("/login", { replace: true });
      }
      throw new Error(errorData.message || "登录已过期");
    }
  }

  // 处理 204 No Content（如删除接口）
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return undefined as T;
  }

  // 解析响应
  const data: ApiResponse<T> = await res.json();

  // 处理业务错误
  if (data.code !== 0) {
    throw new ApiError(data.code, data.message);
  }

      return data.data;
    } finally {
      // 清理去重缓存
      pendingRequests.delete(requestKey);
    }
  })();

  // 缓存 GET 请求
  if (method === "GET") {
    trackPending(requestKey, requestPromise);
  }

  return requestPromise;
}

/**
 * GET 请求
 */
export function get<T = any>(url: string, params?: Record<string, any>): Promise<T> {
  let fullUrl = url;
  if (params) {
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value != null && value !== "") {
        query.append(key, String(value));
      }
    });
    const queryString = query.toString();
    if (queryString) {
      fullUrl += (url.includes("?") ? "&" : "?") + queryString;
    }
  }
  return request<T>(fullUrl, { method: "GET" });
}

/**
 * POST 请求
 */
export function post<T = any>(url: string, body?: any): Promise<T> {
  const isFormData = body instanceof FormData;
  return request<T>(url, {
    method: "POST",
    body: isFormData ? body : (body ? JSON.stringify(body) : undefined),
  });
}

/**
 * PUT 请求
 */
export function put<T = any>(url: string, body?: any): Promise<T> {
  return request<T>(url, {
    method: "PUT",
    body: body ? JSON.stringify(body) : undefined,
  });
}

/**
 * PATCH 请求（Stage 71 PR-C8：与 BFF /users/me + /conversations/:id 对齐）
 */
export function patch<T = any>(url: string, body?: any): Promise<T> {
  return request<T>(url, {
    method: "PATCH",
    body: body ? JSON.stringify(body) : undefined,
  });
}

/**
 * DELETE 请求
 */
export function del<T = any>(url: string): Promise<T> {
  return request<T>(url, { method: "DELETE" });
}

/**
 * 流式请求（SSE）
 * 使用 fetch + ReadableStream 实现，支持 401 自动刷新
 */
export function streamRequest(
  url: string,
  body: any,
  onMessage: (data: any) => void,
  onError?: (error: any) => void
): () => void {
  const controller = new AbortController();
  
  const doFetch = (currentToken: string | null) => {
    const fullUrl = url.startsWith("http") ? url : `${getBaseUrl()}${url}`;
    
    fetch(fullUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": currentToken ? `Bearer ${currentToken}` : "",
      },
      body: JSON.stringify(body),
      credentials: "include",
      signal: controller.signal,
    })
      .then(async (res) => {
        // 处理 401 - Token 过期
        if (res.status === 401) {
          let errorData: ApiResponse;
          try {
            errorData = await res.json();
          } catch {
            errorData = { code: 10003, message: "Token 无效", data: null };
          }
          
          if (errorData.code === 10002) {
            // 尝试刷新 token
            const newToken = await refreshToken();
            if (newToken) {
              // 刷新成功，重试 SSE 请求
              doFetch(newToken);
              return;
            }
          }
          
          // 刷新失败或其他 401
          clearAuth();
          if (import.meta.client) {
            navigateTo("/login", { replace: true });
          }
          throw new Error(errorData.message || "登录已过期");
        }
        
        if (!res.ok) {
          const error = await res.json();
          throw new Error(error.message || "请求失败");
        }
        
        const reader = res.body?.getReader();
        if (!reader) {
          throw new Error("无法读取响应");
        }
        
        const decoder = new TextDecoder();
        let buffer = "";
        
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";
          
          for (const line of lines) {
            if (line.startsWith("data: ")) {
              try {
                const data = JSON.parse(line.slice(6));
                onMessage(data);
              } catch (e) {
                console.warn("解析 SSE 数据失败:", line);
              }
            }
          }
        }
      })
      .catch((error) => {
        onError?.(error);
      });
  };
  
  doFetch(getAccessToken());

  // 返回 abort 函数，供调用方取消请求
  return () => {
    controller.abort();
  };
}

// 注意：ApiError 类已从 ~/types/api 导入复用
