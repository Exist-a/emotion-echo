/**
 * function.test.ts · utils/function.ts 纯函数测试
 *
 * Stage 26-T backlog §五 5.3: 覆盖 deepClone / debounce / throttle /
 * generateId / sleep / isEmpty。
 */
import { describe, it, expect, vi } from "vitest";

import {
  deepClone,
  debounce,
  throttle,
  generateId,
  sleep,
  isEmpty,
} from "./function";

describe("deepClone", () => {
  it("clones a primitive", () => {
    expect(deepClone(42)).toBe(42);
    expect(deepClone("hello")).toBe("hello");
    expect(deepClone(null)).toBe(null);
    expect(deepClone(undefined)).toBe(undefined);
  });

  it("clones a flat object", () => {
    const obj = { a: 1, b: "x", c: true };
    const cloned = deepClone(obj);
    expect(cloned).toEqual(obj);
    expect(cloned).not.toBe(obj); // different reference
  });

  it("clones a nested object", () => {
    const obj = { a: { b: { c: 1 } } };
    const cloned = deepClone(obj) as any;
    expect(cloned).toEqual(obj);
    expect(cloned.a).not.toBe(obj.a);
    expect(cloned.a.b).not.toBe(obj.a.b);
  });

  it("clones an array (preserves elements)", () => {
    const arr = [1, 2, { x: 3 }];
    const cloned = deepClone(arr) as any;
    expect(cloned).toEqual(arr);
    expect(cloned).not.toBe(arr);
    expect(cloned[2]).not.toBe(arr[2]);
  });

  it("clones a Date (independent time)", () => {
    const d = new Date(2024, 0, 1);
    const cloned = deepClone(d);
    expect(cloned).toBeInstanceOf(Date);
    expect(cloned.getTime()).toBe(d.getTime());
    expect(cloned).not.toBe(d);
  });
});

describe("debounce", () => {
  it("delays the call by delay ms", async () => {
    vi.useFakeTimers();
    try {
      const fn = vi.fn();
      const debounced = debounce(fn, 100);
      debounced();
      expect(fn).not.toHaveBeenCalled();
      vi.advanceTimersByTime(99);
      expect(fn).not.toHaveBeenCalled();
      vi.advanceTimersByTime(1);
      expect(fn).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("coalesces multiple rapid calls into one", async () => {
    vi.useFakeTimers();
    try {
      const fn = vi.fn();
      const debounced = debounce(fn, 100);
      debounced();
      debounced();
      debounced();
      vi.advanceTimersByTime(100);
      expect(fn).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("throttle", () => {
  it("fires immediately on first call", () => {
    vi.useFakeTimers();
    try {
      const fn = vi.fn();
      const throttled = throttle(fn, 100);
      throttled();
      expect(fn).toHaveBeenCalledTimes(1);
      // Subsequent calls within limit are dropped.
      throttled();
      throttled();
      expect(fn).toHaveBeenCalledTimes(1);
      vi.advanceTimersByTime(100);
      throttled();
      expect(fn).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("generateId", () => {
  it("returns non-empty string with the given prefix", () => {
    const id = generateId("user");
    expect(id).toMatch(/^user_/);
    expect(id.length).toBeGreaterThan(5);
  });

  it("uses default prefix 'id' when omitted", () => {
    const id = generateId();
    expect(id).toMatch(/^id_/);
  });

  it("returns unique ids across calls", () => {
    const ids = new Set<string>();
    for (let i = 0; i < 100; i++) {
      ids.add(generateId("x"));
    }
    // Some collision is possible (Date.now + random 9 chars), but 100
    // distinct calls should produce > 90 unique values.
    expect(ids.size).toBeGreaterThan(90);
  });
});

describe("sleep", () => {
  it("resolves after the specified delay", async () => {
    const start = Date.now();
    await sleep(50);
    const elapsed = Date.now() - start;
    expect(elapsed).toBeGreaterThanOrEqual(45);
  });
});

describe("isEmpty", () => {
  it.each([
    [null, true],
    [undefined, true],
    ["", true],
    ["   ", true],
    [[], true],
    [{}, true],
    ["hello", false],
    [[1], false],
    [{ a: 1 }, false],
    [0, false],
    [false, false],
  ])("isEmpty(%s) === %s", (input, expected) => {
    expect(isEmpty(input)).toBe(expected);
  });
});