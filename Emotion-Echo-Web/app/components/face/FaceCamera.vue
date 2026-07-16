<template>
  <div
    v-if="visible"
    class="face-camera-wrapper"
    :style="wrapperStyle"
    @mousedown="startDrag"
    @touchstart="startDrag"
  >
    <div class="camera-container">
      <video
        ref="videoRef"
        class="camera-video"
        autoplay
        playsinline
        muted
      ></video>
      <div class="camera-frame"></div>
      <div class="camera-indicator" :class="{ active: isActive }">
        <span class="indicator-dot"></span>
        <span class="indicator-text">{{ isActive ? '面部识别中' : '等待中' }}</span>
      </div>
      <button class="close-btn" @click.stop="handleClose">
        <el-icon><CircleClose /></el-icon>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { CircleClose } from '@element-plus/icons-vue'

interface Props {
  visible: boolean
  isActive?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  isActive: false
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'videoReady', video: HTMLVideoElement): void
}>()

const videoRef = ref<HTMLVideoElement | null>(null)

// 位置状态
const position = ref({ x: 20, y: 20 })
const isDragging = ref(false)
const dragStart = ref({ x: 0, y: 0 })
const elementStart = ref({ x: 0, y: 0 })

// 容器大小
const containerSize = ref({ width: 180, height: 140 })

// 计算样式
const wrapperStyle = computed(() => ({
  left: `${position.value.x}px`,
  top: `${position.value.y}px`,
  width: `${containerSize.value.width}px`,
  height: `${containerSize.value.height}px`
}))

// 监听 visible 变化
watch(() => props.visible, (newVal) => {
  if (newVal) {
    triggerVideoReady()
  }
})

onMounted(() => {
  if (props.visible) {
    triggerVideoReady()
  }
})

const triggerVideoReady = () => {
  if (!videoRef.value) {
    setTimeout(() => {
      if (videoRef.value) {
        emit('videoReady', videoRef.value)
      }
    }, 100)
    return
  }
  emit('videoReady', videoRef.value)
}

// 拖拽相关
const startDrag = (e: MouseEvent | TouchEvent) => {
  isDragging.value = true
  
  const clientX = 'touches' in e ? e.touches[0].clientX : e.clientX
  const clientY = 'touches' in e ? e.touches[0].clientY : e.clientY
  
  dragStart.value = { x: clientX, y: clientY }
  elementStart.value = { ...position.value }
  
  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', stopDrag)
  document.addEventListener('touchmove', onDrag)
  document.addEventListener('touchend', stopDrag)
}

const onDrag = (e: MouseEvent | TouchEvent) => {
  if (!isDragging.value) return
  
  const clientX = 'touches' in e ? e.touches[0].clientX : e.clientX
  const clientY = 'touches' in e ? e.touches[0].clientY : e.clientY
  
  const deltaX = clientX - dragStart.value.x
  const deltaY = clientY - dragStart.value.y
  
  // 限制在可视区域内
  const maxX = window.innerWidth - containerSize.value.width - 10
  const maxY = window.innerHeight - containerSize.value.height - 10
  
  position.value = {
    x: Math.max(10, Math.min(maxX, elementStart.value.x + deltaX)),
    y: Math.max(10, Math.min(maxY, elementStart.value.y + deltaY))
  }
}

const stopDrag = () => {
  isDragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
  document.removeEventListener('touchmove', onDrag)
  document.removeEventListener('touchend', stopDrag)
}

const handleClose = () => {
  emit('close')
}

onUnmounted(() => {
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
  document.removeEventListener('touchmove', onDrag)
  document.removeEventListener('touchend', stopDrag)
})
</script>

<style scoped lang="scss">
.face-camera-wrapper {
  position: fixed;
  z-index: 1000;
  cursor: move;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
  background: #fff;
}

.camera-container {
  width: 100%;
  height: 100%;
  position: relative;
}

.camera-video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  background: #1a1a1a;
}

.camera-frame {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 80%;
  height: 80%;
  border: 2px solid rgba(110, 147, 135, 0.7);
  border-radius: 8px;
  pointer-events: none;
}

.camera-indicator {
  position: absolute;
  bottom: 8px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: rgba(0, 0, 0, 0.6);
  border-radius: 10px;
  font-size: 10px;
  color: var(--color-pencil);

  &.active {
    color: var(--color-sage);

    .indicator-dot {
      background: var(--color-sage);
      animation: blink 1.5s infinite;
    }
  }
}

.indicator-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-ink-soft);
}

@keyframes blink {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.3;
  }
}

.indicator-text {
  white-space: nowrap;
}

.close-btn {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.5);
  color: #fff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  transition: background 0.2s;
  
  &:hover {
    background: rgba(0, 0, 0, 0.7);
  }
  
  :deep(.el-icon) {
    width: 16px;
    height: 16px;
  }
}
</style>
