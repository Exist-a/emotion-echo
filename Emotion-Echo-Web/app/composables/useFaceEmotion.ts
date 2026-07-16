/**
 * 面部情绪分析 Composable
 * 处理摄像头捕获和面部情绪分析逻辑
 */
import { ref, onUnmounted } from 'vue'
import type { FaceEmotionResult } from '~/types/api'
import { useApi } from './useApi'
import { useUserStore } from '~/stores/user'

export interface UseFaceEmotionOptions {
  captureInterval?: number // 捕获间隔（毫秒），默认2000ms
  sessionId?: string
}

export const useFaceEmotion = (options: UseFaceEmotionOptions = {}) => {
  const { post } = useApi()
  const userStore = useUserStore()
  
  const isCameraOn = ref(false)
  const currentEmotion = ref<FaceEmotionResult | null>(null)
  const lastCaptureTime = ref(0)
  
  let stream: MediaStream | null = null
  let videoElement: HTMLVideoElement | null = null
  let captureTimer: ReturnType<typeof setInterval> | null = null
  let canvas: HTMLCanvasElement | null = null
  
  const captureInterval = options.captureInterval || 2000

  /**
   * 开启摄像头
   */
  const startCamera = async (videoRef: HTMLVideoElement) => {
    try {
      videoElement = videoRef
      
      // 请求摄像头权限
      stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: 'user',
          width: { ideal: 320 },
          height: { ideal: 240 }
        }
      })
      
      videoElement.srcObject = stream
      await videoElement.play()
      
      isCameraOn.value = true
      
      // 创建 canvas 用于截图
      if (!canvas) {
        canvas = document.createElement('canvas')
        canvas.width = 320
        canvas.height = 240
      }
      
      // 立即捕获一次
      await captureAndAnalyze()
      
      // 开始定时捕获
      captureTimer = setInterval(async () => {
        await captureAndAnalyze()
      }, captureInterval)
      
      return true
    } catch (error) {
      console.error('[useFaceEmotion] 无法开启摄像头:', error)
      throw new Error('无法访问摄像头，请检查权限设置')
    }
  }

  /**
   * 关闭摄像头
   */
  const stopCamera = () => {
    if (stream) {
      stream.getTracks().forEach(track => track.stop())
      stream = null
    }
    
    if (videoElement) {
      videoElement.srcObject = null
    }
    
    if (captureTimer) {
      clearInterval(captureTimer)
      captureTimer = null
    }
    
    isCameraOn.value = false
    currentEmotion.value = null
    lastCaptureTime.value = 0
  }

  /**
   * 捕获图像并分析情绪
   */
  const captureAndAnalyze = async () => {
    if (!videoElement || !canvas || !isCameraOn.value) return
    
    try {
      const ctx = canvas.getContext('2d')
      if (!ctx) return
      
      // 捕获当前帧
      ctx.drawImage(videoElement, 0, 0, canvas.width, canvas.height)
      
      // 转换为 base64
      const imageBase64 = canvas.toDataURL('image/jpeg', 0.7)
      
      // 获取用户ID
      const userId = userStore.userInfo?.id || ''
      
      // 获取 sessionId（处理可能的 ComputedRef）
      const sessionId = typeof options.sessionId === 'object' && 'value' in options.sessionId 
        ? options.sessionId.value 
        : options.sessionId
      
      // 发送到后端分析
      const result = await post<FaceEmotionResult>('/face/emotion', {
        imageBase64: imageBase64.split(',')[1], // 去掉 data:image/jpeg;base64, 前缀
        sessionId,
        userId
      })
      
      currentEmotion.value = {
        ...result,
        timestamp: Date.now()
      }
      lastCaptureTime.value = Date.now()
      
      // 打印情绪分析结果
      console.log(`[面部情绪] 检测结果: ${result.emotion} (置信度: ${(result.confidence * 100).toFixed(1)}%)`)
      
    } catch (error) {
      console.error('[面部情绪] 分析失败:', error)
    }
  }

  /**
   * 手动触发一次捕获（用于发送消息时）
   */
  const triggerCapture = async () => {
    if (!isCameraOn.value) return null
    await captureAndAnalyze()
    return currentEmotion.value
  }

  /**
   * 获取最近的面部情绪（3秒内有效）
   */
  const getRecentEmotion = () => {
    if (!currentEmotion.value) return null
    const now = Date.now()
    // 3秒内的情绪有效
    if (now - currentEmotion.value.timestamp <= 3000) {
      return currentEmotion.value
    }
    return null
  }

  /**
   * 切换摄像头状态
   */
  const toggleCamera = async (videoRef?: HTMLVideoElement) => {
    if (isCameraOn.value) {
      stopCamera()
    } else if (videoRef) {
      await startCamera(videoRef)
    }
  }

  // 组件卸载时清理
  onUnmounted(() => {
    stopCamera()
  })

  return {
    isCameraOn,
    currentEmotion,
    lastCaptureTime,
    startCamera,
    stopCamera,
    triggerCapture,
    getRecentEmotion,
    toggleCamera
  }
}