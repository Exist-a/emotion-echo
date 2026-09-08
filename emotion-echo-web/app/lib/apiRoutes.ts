// app/lib/apiRoutes.ts
//
// Sprint 1 PR-2 (2026-09-04): 前端 API 路径单点真理
//
// 目的：把散落在 4 composables + 2 stores + 8 pages 的硬编码字符串集中到一处。
//       app/lib/apiRoutes.test.ts 会扫所有 .vue/.ts 文件的字符串字面量，断言 ⊆ 本文件。
//
// 格式约定：
//   - 每条 = { method, path }
//   - 动态路径（:id / :kind 等）保留 BFF 风格（Gin trie 写法），前端在调用点做模板字符串拼接
//   - knownOrphans 段：PR-4 落地前的孤儿路径（前端已调但 BFF 未实现），
//     防止契约测试误报；Sprint 1 PR-4 落地后这些 orphan 应转入主路径或加 BFF handler
//
// 来源：
//   - BFF 路由清单：emotion-echo-web-bff/main.go (PR-1 测试已锁)
//   - 前端现存硬编码：grep 自 composables/stores/pages
//   - todo-pile-2026-09-04.md A2/C8 与 Sprint 1 plan §PR-2

export interface ApiRoute {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
}

export const API_ROUTES = {
  // ============ Auth (BFF: POST /api/v1/auth/:action, switch case 5 个) ============
  authLogin:              { method: 'POST', path: '/auth/login' } as ApiRoute,
  authRegister:           { method: 'POST', path: '/auth/register' } as ApiRoute,
  authLogout:             { method: 'POST', path: '/auth/logout' } as ApiRoute,
  authRefresh:            { method: 'POST', path: '/auth/refresh' } as ApiRoute,
  authVerificationCode:   { method: 'POST', path: '/auth/verification-code' } as ApiRoute,
  // PR-4 落地：authResetPassword + BFF auth_handler case "reset-password" + user-svc /users/reset-password
  authResetPassword:      { method: 'POST', path: '/auth/reset-password' } as ApiRoute,

  // ============ User (BFF: user_handler.go + PR-4 avatar) ============
  userProfile:            { method: 'GET',  path: '/user/profile' } as ApiRoute,
  userUpdateProfile:      { method: 'PUT',  path: '/user/profile' } as ApiRoute,
  // PR-4 落地：BFF avatar_handler + MinIO + user-svc avatar_url
  userAvatar:             { method: 'POST', path: '/user/avatar' } as ApiRoute,

  // ============ Conversations (BFF: chat_handler.go) ============
  conversations:          { method: 'GET',    path: '/conversations' } as ApiRoute,
  createConversation:     { method: 'POST',   path: '/conversations' } as ApiRoute,
  conversationById:       { method: 'PUT',    path: '/conversations/:id' } as ApiRoute,
  pinConversation:        { method: 'POST',   path: '/conversations/:id/pin' } as ApiRoute,
  deleteConversation:     { method: 'DELETE', path: '/conversations/:id' } as ApiRoute,
  messagesByConv:         { method: 'GET',    path: '/conversations/:id/messages' } as ApiRoute,
  sendMessage:            { method: 'POST',   path: '/conversations/:id/messages' } as ApiRoute,

  // ============ Reports / user-behavior / mental-health (BFF: analytics_handler.go) ============
  reportsDaily:           { method: 'GET', path: '/reports/daily' } as ApiRoute,
  reportsTrend:           { method: 'GET', path: '/reports/trend' } as ApiRoute,
  userBehaviorDayNight:   { method: 'GET', path: '/user-behavior/day-night' } as ApiRoute,
  userBehaviorDepth:      { method: 'GET', path: '/user-behavior/depth' } as ApiRoute,
  userBehaviorFrequency:  { method: 'GET', path: '/user-behavior/frequency' } as ApiRoute,

  // ============ Surveys (BFF: survey_handler.go) ============
  surveys:                { method: 'GET',  path: '/surveys' } as ApiRoute,
  surveyById:             { method: 'GET',  path: '/surveys/:id' } as ApiRoute,
  submitSurvey:           { method: 'POST', path: '/surveys/:id/submit' } as ApiRoute,

  // ============ Multimodal / TTS / AI stream (BFF: multimodal/tts/ai_stream handlers) ============
  multimodalAnalyze:      { method: 'POST', path: '/multimodal/analyze' } as ApiRoute,
  ttsStream:              { method: 'POST', path: '/tts/stream' } as ApiRoute,
  aiStream:               { method: 'POST', path: '/ai/stream' } as ApiRoute,

  // PR-4 落地：BFF voice_handler + ai-svc multimodal kind=audio
  voiceUpload:            { method: 'POST', path: '/voice/upload' } as ApiRoute,

  // ============ Uploads（Stage 58 PR-UP-1/2：通用上传）============
  // BFF: POST /api/v1/uploads/:kind（kind ∈ image|video|file）
  // Sprint 1 PR-2 时本路径是 orphan（前端 /upload/* 单数 + BFF /uploads/* 复数错位），
  // Stage 58 PR-UP-1 BFF 真实现 + PR-UP-2 前端转正
  uploadImage:            { method: 'POST', path: '/uploads/image' } as ApiRoute,
  uploadVideo:            { method: 'POST', path: '/uploads/video' } as ApiRoute,
  uploadFile:             { method: 'POST', path: '/uploads/file' } as ApiRoute,

  // ============ knownOrphans ============
  // 前端已调用但 BFF 端未注册（或 BFF 路径错位）。PR-4 落地后这些孤儿应转入主路径或加 handler。
  // 路径写在这里是为了让契约测试不误报，不构成对实现的承诺。
  knownOrphans: {
    // useFaceEmotion.ts 调 /face/emotion — 是死代码，Camera 抓拍走 /multimodal/analyze
    faceEmotionOrphan:  { method: 'POST', path: '/face/emotion' } as ApiRoute,
  },
} as const
