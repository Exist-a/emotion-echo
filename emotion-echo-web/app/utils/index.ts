// utils/index.ts - 统一导出入口

// 日期时间工具
export { formatDate, formatRelativeTime } from './date'

// 通用工具函数
export { deepClone, debounce, throttle, generateId, sleep, isEmpty } from './function'

// 情绪标签工具
export { EMOTION_LABEL_MAP, getEmotionLabel } from './emotion'
export type { EmotionLabel } from './emotion'

// 文件和剪贴板工具
export { formatFileSize, downloadFile, copyToClipboard } from './file'

// URL 工具
export { parseQueryParams, buildQueryString } from './url'

// 安全访问工具
export { safeGet } from './safe'
