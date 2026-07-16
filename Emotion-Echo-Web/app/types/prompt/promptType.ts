export type EmotionType = "happy" | "sad" | "angry" | "anxious" | "neutral";


export interface RCTPromptType {
  role: string;
  context: string;
  task: string;
}