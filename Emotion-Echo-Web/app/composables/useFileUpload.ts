import type { UploadResult, FileType, UploadProgress, FileUploadConfig } from '~/types/api'
import { post } from '~/composables/useApi'

/**
 * 文件上传配置
 */
const UPLOAD_CONFIGS: Record<FileType, FileUploadConfig> = {
  image: {
    maxSize: 5 * 1024 * 1024, // 5MB
    allowedExtensions: ['.jpg', '.jpeg', '.png', '.webp', '.gif']
  },
  file: {
    maxSize: 20 * 1024 * 1024, // 20MB
    allowedExtensions: [] // 允许任意文件
  },
  video: {
    maxSize: 50 * 1024 * 1024, // 50MB
    allowedExtensions: ['.mp4', '.avi', '.mov', '.wmv', '.flv']
  }
}

/**
 * 根据文件扩展名判断文件类型
 */
const getFileType = (filename: string): FileType => {
  const ext = filename.toLowerCase()
  if (UPLOAD_CONFIGS.image.allowedExtensions.some(e => ext.endsWith(e))) {
    return 'image'
  }
  if (UPLOAD_CONFIGS.video.allowedExtensions.some(e => ext.endsWith(e))) {
    return 'video'
  }
  return 'file'
}

/**
 * 获取文件上传端点
 */
const getUploadEndpoint = (type: FileType): string => {
  switch (type) {
    case 'image':
      return '/upload/image'
    case 'video':
      return '/upload/video'
    default:
      return '/upload/file'
  }
}

/**
 * 格式化文件大小
 */
const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
}

export function useFileUpload() {
  const isUploading = ref(false)
  const uploadProgress = ref<UploadProgress | null>(null)
  const error = ref<string | null>(null)

  /**
   * 验证文件
   */
  const validateFile = (file: File, type: FileType): { valid: boolean; error?: string } => {
    const config = UPLOAD_CONFIGS[type]

    // 验证大小
    if (file.size > config.maxSize) {
      return {
        valid: false,
        error: `文件大小超过限制，最大允许 ${formatFileSize(config.maxSize)}`
      }
    }

    // 验证类型
    if (config.allowedExtensions.length > 0) {
      const ext = '.' + file.name.split('.').pop()?.toLowerCase()
      if (!ext || !config.allowedExtensions.includes(ext)) {
        return {
          valid: false,
          error: `不支持的文件类型，仅支持: ${config.allowedExtensions.join(', ')}`
        }
      }
    }

    return { valid: true }
  }

  /**
   * 上传单个文件
   */
  const uploadFile = async (file: File, type?: FileType): Promise<UploadResult & { type: FileType }> => {
    isUploading.value = true
    error.value = null
    uploadProgress.value = null

    try {
      // 自动判断文件类型
      const fileType = type || getFileType(file.name)

      // 验证文件
      const validation = validateFile(file, fileType)
      if (!validation.valid) {
        throw new Error(validation.error)
      }

      // 创建 FormData
      const formData = new FormData()
      formData.append('file', file)

      // 上传
      const result = await post<UploadResult>(getUploadEndpoint(fileType), formData)

      return {
        ...result,
        type: fileType
      }
    } catch (err: any) {
      error.value = err.message || '上传失败'
      throw err
    } finally {
      isUploading.value = false
      uploadProgress.value = null
    }
  }

  /**
   * 上传多个文件
   */
  const uploadFiles = async (files: FileList | File[]): Promise<Array<UploadResult & { type: FileType }>> => {
    const fileArray = Array.from(files)
    const results: Array<UploadResult & { type: FileType }> = []

    for (const file of fileArray) {
      const result = await uploadFile(file)
      results.push(result)
    }

    return results
  }

  return {
    isUploading,
    uploadProgress,
    error,
    uploadFile,
    uploadFiles,
    validateFile,
    getFileType,
    formatFileSize,
    UPLOAD_CONFIGS
  }
}
