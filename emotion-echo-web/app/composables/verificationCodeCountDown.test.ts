/**
 * verificationCodeCountDown.test.ts · composables/verificationCodeCountDown.ts
 *
 * Stage 26-T backlog §五 5.4: 覆盖 60 秒倒计时 composable。
 *
 * 策略：
 *   - 用 vi.useFakeTimers() 控制 setInterval 触发。
 *   - 每个 test 后清理 interval（避免 vitest worker 跨 test 污染）。
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

import { verificationCodeCountDown } from "./verificationCodeCountDown";

describe("verificationCodeCountDown", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns refs and helpers", () => {
    const c = verificationCodeCountDown();
    expect(c.isGetVerificationCode.value).toBe(false);
    expect(c.lastSeconds.value).toBe(60);
    expect(c.verificationCodeText).toBe("秒后重新获取");
    expect(typeof c.startCountdown).toBe("function");
    expect(typeof c.stopCountdown).toBe("function");
  });

  it("startCountdown flips isGetVerificationCode and resets to 60s", () => {
    const c = verificationCodeCountDown();
    c.lastSeconds.value = 10; // pre-set
    c.startCountdown();
    expect(c.isGetVerificationCode.value).toBe(true);
    expect(c.lastSeconds.value).toBe(60);
  });

  it("decrements lastSeconds every second", () => {
    const c = verificationCodeCountDown();
    c.startCountdown();
    expect(c.lastSeconds.value).toBe(60);
    vi.advanceTimersByTime(1000);
    expect(c.lastSeconds.value).toBe(59);
    vi.advanceTimersByTime(1000);
    expect(c.lastSeconds.value).toBe(58);
  });

  it("stops countdown + resets flag at 0", () => {
    const c = verificationCodeCountDown();
    c.startCountdown();
    vi.advanceTimersByTime(60_000); // exactly 60 ticks
    // After the 60th tick lastSeconds becomes 0 → clear + flag=false.
    expect(c.isGetVerificationCode.value).toBe(false);
    // lastSeconds should be 0 (or possibly -1 depending on impl;
    // we pin current behavior: <=0 condition clears interval).
    expect(c.lastSeconds.value).toBeLessThanOrEqual(0);
  });

  it("stopCountdown clears the interval early", () => {
    const c = verificationCodeCountDown();
    c.startCountdown();
    vi.advanceTimersByTime(5000); // 5s elapsed
    expect(c.lastSeconds.value).toBe(55);
    c.stopCountdown();
    expect(c.isGetVerificationCode.value).toBe(false);
    vi.advanceTimersByTime(10_000); // would otherwise tick 10 more
    // Must not have changed after stop.
    expect(c.lastSeconds.value).toBe(55);
  });

  it("restarting countdown resets to 60 (no interval stacking)", () => {
    const c = verificationCodeCountDown();
    c.startCountdown();
    vi.advanceTimersByTime(20_000);
    expect(c.lastSeconds.value).toBe(40);
    c.startCountdown(); // restart
    expect(c.lastSeconds.value).toBe(60);
    vi.advanceTimersByTime(1000);
    expect(c.lastSeconds.value).toBe(59); // single tick, not 41 (no stacking)
  });
});