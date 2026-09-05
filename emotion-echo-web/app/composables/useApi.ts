// composables/useApi.ts - API 请求封装
import type { ApiResponse } from "~/types/api";
import { ApiError } from "~/types/api";
import { useUserStore } from "~/stores/user";
import { navigateTo } from "#app";

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
function getBaseUrl(): string {
  try {
    const config = useRuntimeConfig();
    return (config.public.API_BASE_URL as string) || "http://localhost:8080/api/v1";
  } catch {
    return "http://localhost:8080/api/v1";
  }
}

// 429 限流配置
const MAX_RETRY_COUNT = 2; // 减少重试次数
const DEFAULT_RETRY_DELAY_MS = 2000; // 延长退避基准

// Token 刷新锁，防止并发刷新
let refreshPromise: Promise<string | null> | null = null;

// 请求去重：记录正在进行的请求
const pendingRequests = new Map<string, Promise<any>>();

// 生成请求唯一标识
function getRequestKey(url: string, options: RequestInit = {}): string {
  const method = options.method || "GET";
  const body = options.body ? (typeof options.body === "string" ? options.body : JSON.stringify(options.body)) : "";
  return `${method}:${url}:${body}`;
}

/**
 * 获取 AccessToken
 * SSR 时从 cookie 读取，CSR 时从 userStore 读取
 */
function getAccessToken(): string | null {
  if (import.meta.client) {
    const userStore = useUserStore();
    return userStore.getAccessToken || null;
  }
  // SSR 时从 cookie 读取
  const tokenCookie = useCookie("access_token");
  return tokenCookie.value || null;
}

/**
 * 设置 AccessToken（通过 userStore，统一处理 rememberMe 逻辑）
 */
function setAccessToken(token: string, expiresIn: number = 900, rememberMe?: boolean): void {
  const userStore = useUserStore();
  // 如果未传入 rememberMe，根据现有存储推断
  if (rememberMe === undefined) {
    rememberMe = import.meta.client ? !!localStorage.getItem("access_token") : false;
  }
  userStore.setAccessToken(token, expiresIn, rememberMe);
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

      const res = await fetch(`${getBaseUrl()}/auth/refresh`, {
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
      let res = await fetch(fullUrl, {
        ...options,
        headers,
        credentials: "include", // 携带 Cookie
      });

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
    pendingRequests.set(requestKey, requestPromise);
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
