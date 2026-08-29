/**
 * digitalHuman.test.ts · stores/digitalHuman.ts Pinia store 测试
 *
 * Stage 26-T backlog §五 5.4: 覆盖数字人 store 的 state + actions。
 *
 * 策略：用 @vue/test-utils + Pinia 测试 store — setActivePinia 注入
 * 一个新的 Pinia 实例，避免污染全局 store 状态。
 */
import { describe, it, expect, beforeEach } from "vitest";
import { setActivePinia, createPinia } from "pinia";

import { useDigitalHumanStore } from "./digitalHuman";

describe("useDigitalHumanStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("default state", () => {
    const s = useDigitalHumanStore();
    expect(s.visible).toBe(true);
    expect(s.position).toEqual({ x: 0, y: 0 });
    expect(s.voiceEnabled).toBe(true);
    expect(s.currentLipShape).toBe("neutral");
    expect(s.volume).toBe(2.0);
  });

  it("setPosition updates x/y", () => {
    const s = useDigitalHumanStore();
    s.setPosition(120, 80);
    expect(s.position).toEqual({ x: 120, y: 80 });
  });

  it("toggleVisible flips visible", () => {
    const s = useDigitalHumanStore();
    expect(s.visible).toBe(true);
    s.toggleVisible();
    expect(s.visible).toBe(false);
    s.toggleVisible();
    expect(s.visible).toBe(true);
  });

  it("toggleVoice flips voiceEnabled", () => {
    const s = useDigitalHumanStore();
    expect(s.voiceEnabled).toBe(true);
    s.toggleVoice();
    expect(s.voiceEnabled).toBe(false);
  });

  it("setVisible overrides visible", () => {
    const s = useDigitalHumanStore();
    s.setVisible(false);
    expect(s.visible).toBe(false);
    s.setVisible(true);
    expect(s.visible).toBe(true);
  });

  it("setLipShape updates currentLipShape", () => {
    const s = useDigitalHumanStore();
    s.setLipShape("happy_open");
    expect(s.currentLipShape).toBe("happy_open");
  });

  it("setVolume updates volume", () => {
    const s = useDigitalHumanStore();
    s.setVolume(5.5);
    expect(s.volume).toBe(5.5);
  });

  it("independent instances via different Pinia contexts", () => {
    const a = useDigitalHumanStore();
    a.setPosition(1, 1);
    // New Pinia in the same test should give a fresh instance.
    setActivePinia(createPinia());
    const b = useDigitalHumanStore();
    expect(b.position).toEqual({ x: 0, y: 0 });
    expect(a.position).toEqual({ x: 1, y: 1 });
  });
});