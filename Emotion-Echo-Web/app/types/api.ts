// types/api.ts - API 类型定义

/**
 * 通用 API 响应
 */
export interface ApiResponse<T = any> {
  code: number
  message: string
  data: T
}

/**
 * API 错误
 */
export class ApiError extends Error {
  code: number

  constructor(code: number, message: string) {
    super(message)
    this.code = code
    this.name = 'ApiError'
  }
}

// ==================== 认证模块 ====================

/**
 * 发送验证码请求
 */
export interface SendVerificationCodeParams {
  username: string // 手机号或邮箱
  type: 'register' | 'login' | 'reset'
}

/**
 * 重置密码请求
 */
export interface ResetPasswordParams {
  username: string // 手机号或邮箱
  verificationCode: string // 6位验证码
  newPassword: string // SHA256 哈希后的新密码
}

/**
 * 登录请求
 */
export interface LoginParams {
  username: string
  password: string // SHA256 哈希后的密码
  rememberMe?: boolean // 记住我
}

/**
 * 注册请求
 */
export interface RegisterParams extends LoginParams {
  verificationCode: string
}

/**
 * 登录响应数据
 */
export interface LoginData {
  accessToken: string
  expiresIn: number
  user: UserInfo
}

// ==================== 用户模块 ====================

/**
 * 用户信息
 */
export interface UserInfo {
  id: string
  username: string
  nickname: string
  avatar: string
  age: number | null
  config: {
    fontSize?: 'small' | 'medium' | 'large' | '14px' | '16px' | '18px'
    theme?: 'light' | 'dark' | 'auto'
  }
  createdAt: string
  updatedAt?: string
}

/**
 * 更新用户资料请求
 */
export interface UpdateProfileParams {
  nickname?: string
  age?: number
  config?: {
    fontSize?: 'small' | 'medium' | 'large' | '14px' | '16px' | '18px'
    theme?: 'light' | 'dark' | 'auto'
  }
}

// ==================== 会话模块 ====================

/**
 * 会话项
 */
export interface ConversationItem {
  id: string
  userId: string
  title: string
  isTop: boolean
  lastMessage: string | null
  lastMessageTime: number | null
  createdAt: string
  updatedAt: string
}

/**
 * 创建会话请求
 */
export interface CreateConversationParams {
  title?: string
}

// ==================== 消息模块 ====================

/**
 * 消息项
 */
export interface MessageItem {
  id: string
  conversationId: string
  sender: 'user' | 'ai'
  content: string
  contentType: 'text' | 'audio' | 'img'
  emotionTag?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
  sendTime: number
  createdAt: number
  audioUrl?: string
  audioDuration?: number
}

/**
 * 带状态的消息
 */
export interface MessageWithStatus extends MessageItem {
  status?: 'sending' | 'sent' | 'streaming'
}

/**
 * 发送消息请求
 */
export interface SendMessageParams {
  content: string
  contentType?: 'text' | 'audio' | 'img'
  emotionTag?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
}

// ==================== AI 模块 ====================

/**
 * AI 流式请求
 */
export interface AIStreamParams {
  conversationId?: string
  message: string
  emotion?: "happy" | "sad" | "angry" | "anxious" | "neutral"
  model?: string
}

/**
 * AI 流式响应块
 */
export interface StreamChunk {
  type: 'start' | 'delta' | 'finish' | 'error'
  conversationId?: string
  userMessageId?: string
  content?: string
  messageId?: string
  code?: number
  message?: string
  error?: string
  emotion?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
}

// ==================== 测验模块 ====================

/**
 * 量表项
 */
export interface SurveyItem {
  id: number
  title: string
  description: string
  estimatedTime: string
  status: 'completed' | 'not_started'
  completedAt?: string
  resultId?: string
}

/**
 * 题目选项
 */
export interface QuestionOption {
  id: number
  text: string
  score: number
}

/**
 * 题目
 */
export interface Question {
  id: number
  title: string
  type: 'radio'
  options: QuestionOption[]
}

/**
 * 量表详情
 */
export interface SurveyDetail {
  id: number
  title: string
  description: string
  estimatedTime: string
  questions: Question[]
}

/**
 * 提交答案请求
 */
export interface SubmitSurveyParams {
  answers: Array<{
    questionId: number
    optionId: number
  }>
}

/**
 * 测验结果
 */
export interface SurveyResult {
  resultId: string
  totalScore: number
  level: string
  suggestion: string
}

// ==================== 报表模块 ====================

/**
 * 情绪分布
 */
export interface EmotionDistribution {
  name: string
  value: number
}

/**
 * 日报数据
 */
export interface DailyReport {
  date: string
  summary: string
  emotionDistribution: EmotionDistribution[]
  conversationCount: number
  messageCount: number
  wordCount: number
}

/**
 * 情绪趋势
 */
export interface EmotionTrend {
  type: 'daily' | 'weekly' | 'monthly'
  dates: string[]
  series: Array<{
    name: string
    data: number[]
  }>
  summary: string
  emotionDistribution: EmotionDistribution[]
  conversationCount: number
  messageCount: number
  wordCount: number
}
