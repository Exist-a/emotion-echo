/**
 * 消息意图标签工具（Stage 82 PR-3b）
 *
 * 6 类意图定义与后端 emotion-llm-service intent.INTENTS /
 * chat-svc allowedIntents 三方一致。
 */

export type IntentLabel =
  | "emotional_support"
  | "study_help"
  | "tech_help"
  | "career_help"
  | "lifestyle"
  | "other"
  | "unk";

export const INTENT_LABEL_MAP: Record<IntentLabel, string> = {
  emotional_support: "情感疏导",
  study_help: "学习问题",
  tech_help: "技术问题",
  career_help: "职业问题",
  lifestyle: "生活问题",
  other: "其他",
  unk: "未分类",
};

export function getIntentLabel(intent: string): string {
  return INTENT_LABEL_MAP[intent as IntentLabel] ?? INTENT_LABEL_MAP.unk;
}
