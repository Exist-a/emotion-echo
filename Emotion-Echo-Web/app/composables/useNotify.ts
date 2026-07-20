// composables/useNotify.ts
// 极简原生 toast 通知（替代 Element Plus 的 ElNotification / ElMessage）。
// 在客户端渲染一个固定定位的列表，自动消失。
interface Toast {
  id: number
  title: string
  message: string
  type: 'success' | 'error' | 'warning' | 'info'
  duration: number
}

const toasts = ref<Toast[]>([])
let seq = 0

function push(title: string, message: string, type: Toast['type'] = 'info', duration = 3000) {
  if (!import.meta.client) return
  const id = ++seq
  toasts.value.push({ id, title, message, type, duration })
  window.setTimeout(() => {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }, duration)
}

export function useNotify() {
  return {
    toasts,
    success: (title: string, message = '') => push(title, message, 'success'),
    error: (title: string, message = '') => push(title, message, 'error'),
    warning: (title: string, message = '') => push(title, message, 'warning'),
    info: (title: string, message = '') => push(title, message, 'info'),
    // 兼容之前 ElNotification(title, message, type) 风格
    show: (title: string, message: string, type: Toast['type'] = 'info', duration = 3000) => push(title, message, type, duration),
    push
  }
}

// 全局短调用：在 <script setup> 之外也能用
export function notify(title: string, message: string, type: Toast['type'] = 'info', duration = 3000) {
  push(title, message, type, duration)
}
