// app/lib/apiBaseUrl.ts
//
// PR-A · decision 18 #24 (2026-09-10) — 前端 API base URL fail-fast helper
//
// 目的：把"读 NUXT_PUBLIC_API_BASE_URL"集中到一处；当未配置时抛错而非静默
//       回退到死端口（决策 18 #24 登记的 3 处 fallback 字面值 8894 / 8080 bug）。
//
// 来源：
//   - emotion-echo-web/nuxt.config.ts:19 默认 'http://localhost:19080/api/v1'
//   - 决策 11/12：APISIX 是唯一业务入口，BFF / 业务 svc 不应被前端直连
//   - doc-drift-registry.md #24

import type { RuntimeConfig } from 'nuxt/schema'

/**
 * 读 runtimeConfig.public.API_BASE_URL；未配 / 空串 / NaN 时抛错。
 *
 * 抛错策略（fail-fast）：
 *   - 任何静默回退到 localhost:NNNN 都会让 dev 误以为"端口没人监听 = 后端没起"
 *     而忽略真正原因（.env 漏配或 NUXT_PUBLIC_API_BASE_URL 拼错）
 *   - 改为抛错后，dev 第一时间看到 stack trace 提示环境变量问题
 *
 * 兜底保留 dev 默认值：在 nuxt.config.ts 里 `process.env.X || "http://localhost:19080/api/v1"`
 * 已经覆盖——helper 只负责"配置已注入后是否可用"。
 */
export function getApiBaseUrl(config?: RuntimeConfig): string {
  // 允许外部传 config（单测）；不传时尝试从 Nuxt 全局拿
  let publicCfg: any
  try {
    publicCfg = config?.public ?? (globalThis as any).useRuntimeConfig?.()?.public
  } catch {
    publicCfg = undefined
  }
  const url = publicCfg?.API_BASE_URL as string | undefined
  if (!url || typeof url !== 'string' || url.trim() === '') {
    throw new Error(
      '[apiBaseUrl] NUXT_PUBLIC_API_BASE_URL 未配置 — ' +
        '请设置 .env / .env.example 里的 NUXT_PUBLIC_API_BASE_URL=http://localhost:19080/api/v1 ' +
        '（决策 11/12：APISIX 是唯一业务入口）。' +
        '决策 18 doc-drift-registry.md #24。'
    )
  }
  return url
}