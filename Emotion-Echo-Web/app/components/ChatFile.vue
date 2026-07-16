<template>
  <div class="chat-file" @click="handleClick">
    <!-- 图片消息 -->
    <div v-if="contentType === 'image'" class="chat-image">
      <img :src="getFullUrl(content)" :alt="filename" @error="handleImageError" />
    </div>

    <!-- 视频消息 -->
    <div v-else-if="contentType === 'video'" class="chat-video">
      <video :src="getFullUrl(content)" controls :poster="getVideoPoster" />
    </div>

    <!-- 通用文件消息 -->
    <div v-else class="chat-file-box">
      <div class="file-icon">
        <svg v-if="contentType === 'file'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" />
          <polyline points="13 2 13 9 20 9" />
        </svg>
      </div>
      <div class="file-info">
        <div class="file-name">{{ filename }}</div>
        <div class="file-size">{{ formatSize }}</div>
      </div>
      <div class="download-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="7 10 12 15 17 10" />
          <line x1="12" y1="15" x2="12" y2="3" />
        </svg>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  content: string
  contentType: 'image' | 'file' | 'video'
  filename?: string
  size?: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  download: [url: string]
}>()

const runtimeConfig = useRuntimeConfig()

/**
 * 获取完整 URL
 */
const getFullUrl = (url: string): string => {
  if (!url) return ''
  if (url.startsWith('http')) return url
  const apiBase = runtimeConfig.public.API_BASE_URL as string
  const baseUrl = apiBase.replace('/api/v1', '')
  return baseUrl + url
}

/**
 * 从 URL 提取文件名
 */
const filename = computed(() => {
  if (props.filename) return props.filename
  try {
    const url = getFullUrl(props.content)
    const parts = url.split('/')
    return parts[parts.length - 1] || 'file'
  } catch {
    return 'file'
  }
})

/**
 * 格式化文件大小
 */
const formatSize = computed(() => {
  if (!props.size) return ''
  const bytes = props.size
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
})

/**
 * 获取视频缩略图（使用第一帧）
 */
const getVideoPoster = computed(() => {
  return '' // 可以后续添加视频缩略图逻辑
})

/**
 * 处理点击（下载）
 */
const handleClick = () => {
  const url = getFullUrl(props.content)
  window.open(url, '_blank')
  emit('download', url)
}

/**
 * 处理图片加载失败
 */
const handleImageError = (e: Event) => {
  const img = e.target as HTMLImageElement
  img.style.display = 'none'
  // 可以添加一个占位图
}
</script>

<style scoped lang="scss">
.chat-file {
  cursor: pointer;
  transition: opacity 0.2s;

  &:hover {
    opacity: 0.8;
  }
}

.chat-image {
  max-width: 300px;
  border-radius: 8px;
  overflow: hidden;

  img {
    width: 100%;
    height: auto;
    display: block;
    border-radius: 8px;
  }
}

.chat-video {
  max-width: 400px;
  border-radius: 8px;
  overflow: hidden;

  video {
    width: 100%;
    display: block;
    border-radius: 8px;
  }
}

.chat-file-box {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(255, 255, 255, 0.8);
  border-radius: 8px;
  gap: 12px;
  min-width: 200px;

  .file-icon {
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--color-mist);
    border-radius: 8px;
    color: var(--color-sage-deep);

    svg {
      width: 24px;
      height: 24px;
    }
  }

  .file-info {
    flex: 1;
    min-width: 0;

    .file-name {
      font-size: 14px;
      color: var(--color-ink);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .file-size {
      font-size: 12px;
      color: var(--color-pencil);
      margin-top: 4px;
    }
  }

  .download-icon {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-sage-deep);

    svg {
      width: 20px;
      height: 20px;
    }
  }
}
</style>
