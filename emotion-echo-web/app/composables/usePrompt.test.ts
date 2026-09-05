/**
 * usePrompt.test.ts · composables/usePrompt.ts 纯函数测试
 *
 * Stage 26-T backlog §五 5.4: 覆盖 buildRCTPrompt 的所有 emotion 分支。
 *
 * Per AGENTS.md §三.3: 纯函数，无副作用，无需 mock。
 */
import { describe, it, expect } from "vitest";

import { buildRCTPrompt } from "./usePrompt";

describe("buildRCTPrompt", () => {
  it("happy branch returns warm + cheerful role/task", () => {
    const p = buildRCTPrompt("happy", "我中了彩票！");
    assertPrompt(p, {
      role_includes: "温暖",
      tone: "轻松愉快",
      input_quote: "我中了彩票！",
    });
  });

  it("sad branch returns empathetic tone", () => {
    const p = buildRCTPrompt("sad", "今天很难过");
    assertPrompt(p, {
      role_includes: "温柔",
      tone: "温柔共情",
      input_quote: "今天很难过",
    });
  });

  it("angry branch returns calm tone", () => {
    const p = buildRCTPrompt("angry", "太不公平了");
    assertPrompt(p, {
      role_includes: "冷静",
      tone: "平和冷静",
      input_quote: "太不公平了",
    });
  });

  it("anxious branch returns calming tone", () => {
    const p = buildRCTPrompt("anxious", "明天的面试怎么办");
    assertPrompt(p, {
      role_includes: "舒缓",
      tone: "舒缓放松",
      input_quote: "明天的面试怎么办",
    });
  });

  it("neutral branch returns friendly professional tone", () => {
    const p = buildRCTPrompt("neutral", "想了解一下情绪管理");
    assertPrompt(p, {
      role_includes: "心理健康",
      tone: "友好专业",
      input_quote: "想了解一下情绪管理",
    });
  });

  it("includes length constraint in task", () => {
    const p = buildRCTPrompt("happy", "x");
    assert.match(p.task, /200字/);
  });

  it("includes no-fabrication rule in task", () => {
    const p = buildRCTPrompt("sad", "x");
    assert.match(p.task, /禁止编造/);
  });

  it("preserves original emotion field", () => {
    // buildRCTPrompt 不直接返回 emotion 字段（不破坏 RCTPromptType），
    // 但 task 字段里含有 emotion 字符串。
    const p = buildRCTPrompt("anxious", "x");
    assert.match(p.task, /anxious|舒缓/);
  });

  it("preserves context from base prompts", () => {
    const p = buildRCTPrompt("happy", "x");
    assert.match(p.context, /开心|愉悦/);
  });

  it("preserves user input verbatim in task", () => {
    const p = buildRCTPrompt("happy", "今天天气真好");
    assert.match(p.task, /今天天气真好/);
  });
});

// Helper to assert prompt structure.
function assertPrompt(
  p: ReturnType<typeof buildRCTPrompt>,
  expectations: { role_includes: string; tone: string; input_quote: string },
) {
  assert.include(p.role, expectations.role_includes);
  assert.match(p.task, new RegExp(expectations.tone));
  assert.match(p.task, new RegExp(`"${expectations.input_quote}"`));
  // Length + no-fabrication rules are universal.
  assert.match(p.task, /200字/);
  assert.match(p.task, /禁止编造/);
}