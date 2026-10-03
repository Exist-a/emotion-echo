/**
 * E2E-27 M1 / F-116（ADR-2026-09 决策 1）：对象 URL 解析
 *
 * 服务端下发的对象 URL 现为网关相对路径（/api/v1/...），浏览器渲染 <img>/<video>
 * 前必须解析成网关绝对地址。语义与 ChatFile.getFullUrl / getFullAudioUrl 一致
 * （抽成共享 util，头像链接入；后两者暂不动——已验证可用，避免波及语音/附件链）。
 *
 * - `/api/...` 相对路径 → 拼网关 origin（apiBase 去掉 /api/v1 前缀，避免重复）
 * - `http(s)://` legacy 绝对地址 → 原样（存量 DB 数据不回填，惰性兼容）
 * - 其余（/imgs 静态资源、blob: 本地预览）→ 原样
 */
export function resolveObjectUrl(
  url: string | null | undefined,
  apiBase: string,
): string {
  if (!url) return ''
  if (url.startsWith('/api/')) {
    const baseUrl = (apiBase || '').replace('/api/v1', '')
    return baseUrl + url
  }
  return url
}
