/**
 * date.test.ts · utils/date.ts 格式化函数测试
 *
 * Stage 26-T backlog §五 5.3: 覆盖 formatDate + formatRelativeTime。
 *
 * 策略：用 vi.setSystemTime 固定"现在"，避免相对时间函数不稳定。
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

import { formatDate, formatRelativeTime } from "./date";

// Fixed reference moment: 2024-06-15 12:30:45 UTC+8 (CST)
const REFERENCE = new Date(2024, 5, 15, 12, 30, 45);

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(REFERENCE);
});
afterEach(() => {
  vi.useRealTimers();
});

describe("formatDate", () => {
  it("formats with default pattern YYYY-MM-DD HH:mm", () => {
    const out = formatDate(REFERENCE);
    expect(out).toBe("2024-06-15 12:30");
  });

  it("supports custom pattern (YYYY/MM/DD)", () => {
    const out = formatDate(REFERENCE, "YYYY/MM/DD");
    expect(out).toBe("2024/06/15");
  });

  it("supports seconds pattern (HH:mm:ss)", () => {
    const out = formatDate(REFERENCE, "HH:mm:ss");
    expect(out).toBe("12:30:45");
  });

  it("accepts a number (epoch ms)", () => {
    const out = formatDate(REFERENCE.getTime(), "YYYY-MM-DD");
    expect(out).toBe("2024-06-15");
  });

  it("accepts a string date", () => {
    const out = formatDate("2024-06-15T12:30:45", "YYYY-MM-DD");
    expect(out).toBe("2024-06-15");
  });

  it("zero-pads single-digit month and day", () => {
    const d = new Date(2024, 0, 5, 9, 5, 0);
    expect(formatDate(d, "YYYY-MM-DD HH:mm:ss")).toBe("2024-01-05 09:05:00");
  });
});

describe("formatRelativeTime", () => {
  it('returns "刚刚" for < 1 minute ago', () => {
    const target = new Date(REFERENCE.getTime() - 30 * 1000); // 30s ago
    expect(formatRelativeTime(target)).toBe("刚刚");
  });

  it("returns minutes for < 1 hour ago", () => {
    const target = new Date(REFERENCE.getTime() - 5 * 60 * 1000);
    expect(formatRelativeTime(target)).toBe("5分钟前");
  });

  it("returns hours for < 1 day ago", () => {
    const target = new Date(REFERENCE.getTime() - 3 * 60 * 60 * 1000);
    expect(formatRelativeTime(target)).toBe("3小时前");
  });

  it("returns days for < 1 week ago", () => {
    const target = new Date(REFERENCE.getTime() - 2 * 24 * 60 * 60 * 1000);
    expect(formatRelativeTime(target)).toBe("2天前");
  });

  it("returns formatted date for ≥ 1 week ago", () => {
    const target = new Date(REFERENCE.getTime() - 14 * 24 * 60 * 60 * 1000);
    expect(formatRelativeTime(target)).toBe("2024-06-01");
  });

  it("accepts a number (epoch ms)", () => {
    const target = REFERENCE.getTime() - 60 * 1000;
    expect(formatRelativeTime(target)).toBe("1分钟前");
  });
});