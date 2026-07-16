/**
 * 情绪标签工具函数
 */

export type EmotionLabel = "happy" | "sad" | "angry" | "anxious" | "neutral" | "unk";

export const EMOTION_LABEL_MAP: Record<EmotionLabel, string> = {
  happy: "开心",
  sad: "悲伤",
  angry: "愤怒",
  anxious: "焦虑",
  neutral: "中性",
  unk: "未知",
};

export function getEmotionLabel(emotion: string): string {
  return EMOTION_LABEL_MAP[emotion as EmotionLabel] ?? emotion;
}
