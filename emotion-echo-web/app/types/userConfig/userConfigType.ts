// types/userConfig/userConfigType.ts - 用户配置类型
//
// E2E-12「契约漂移清理」①：fontSize 此前只有一种类型且写的是 px
// （`'14px' | '16px' | '18px' | '20px'`），但 UI 层（setting.vue 的按钮、
// store 对外暴露的 getUserConfig）实际流通的是**语义名**，两边靠 `as any`
// 与宽 union 含糊过去。此处把两种形态显式分开，各自有唯一含义：
//
//   - `fontSizePxType`  —— wire 形态：服务端 `users.config` 的存储值（写库/读库）
//   - `fontSizeType`    —— UI 形态：组件与 store 对外流通的语义名
//
// 转换只在 store 内一处发生（`fontSizeToPx` / `pxToFontSize`）。

/** UI 形态：语义名。setting.vue 的按钮取值、getUserConfig() 的返回形态 */
export type fontSizeType = 'small' | 'medium' | 'large'

/** wire 形态：服务端 users.config 存储的 px 值 */
export type fontSizePxType = '14px' | '16px' | '18px'

export type themeType = 'light' | 'dark' | 'auto'

/** UI 形态：数字人语音语速三档（设置页取值）。F-199 后续（2026-10-07 用户反馈 0.75 偏慢） */
export type ttsSpeedType = 'slow' | 'normal' | 'fast'

/** 语义档 → 播放倍速（= 请求体 speed：BFF clamp [0.25,4] 透传上游 + 前端 playbackRate） */
export const TTS_SPEED_TO_VALUE: Record<ttsSpeedType, number> = {
  slow: 0.75,
  normal: 1,
  fast: 1.25,
}

/**
 * 用户配置（UI 形态）
 */
export interface userConfigType {
  fontSize: fontSizeType
  theme?: themeType
  ttsSpeed?: ttsSpeedType
}

/**
 * 用户配置请求参数（wire 形态：发给服务端的是 px）
 */
export interface UpdateUserConfigParams {
  fontSize?: fontSizePxType
  theme?: themeType
  ttsSpeed?: ttsSpeedType
}
