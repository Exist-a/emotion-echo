import type { EmotionType, RCTPromptType } from "~/types/prompt/promptType";

export function buildRCTPrompt(
  emotion: EmotionType,
  userInput: string,
): RCTPromptType {
  const basePrompts: Record<EmotionType, Omit<RCTPromptType, "task">> = {
    happy: {
      role: "温暖积极的对话伙伴",
      context: "用户正处于开心、愉悦的情绪中，可以分享他们的喜悦，并给予积极的回应和鼓励",
    },
    sad: {
      role: "温柔耐心的倾听者和情绪疏导者",
      context: "用户正处于低落、悲伤的情绪中，需要被理解和安慰",
    },
    angry: {
      role: "冷静客观的引导者",
      context: "用户正处于愤怒、暴躁的情绪中，需要被平复和理性分析",
    },
    anxious: {
      role: "舒缓放松的陪伴者",
      context: "用户正处于焦虑、紧张的情绪中，需要被安抚和提供可行的建议",
    },
    neutral: {
      role: "专业的心理健康助手",
      context: "用户情绪平静，可以进行常规的心理健康咨询和建议",
    },
  };

  const base = basePrompts[emotion];

  return {
    ...base,
    task: `请基于用户输入："${userInput}"，以${base.role}的身份进行回应。要求：
1.  语气必须${emotion === "sad" ? "温柔共情" : emotion === "angry" ? "平和冷静" : emotion === "anxious" ? "舒缓放松" : emotion === "happy" ? "轻松愉快" : "友好专业"}，避免使用专业术语；
2.  回复长度控制在200字以内；
3.  禁止编造信息，若无法提供有效帮助，请明确告知用户。`,
  };
}
