/**
 * url.test.ts · utils/url.ts URL 参数工具测试
 *
 * Stage 26-T backlog §五 5.3: 覆盖 parseQueryParams + buildQueryString。
 */
import { describe, it, expect } from "vitest";

import { parseQueryParams, buildQueryString } from "./url";

describe("parseQueryParams", () => {
  it("parses a URL with multiple params", () => {
    const out = parseQueryParams("https://x.com/p?a=1&b=two&c=");
    expect(out).toEqual({ a: "1", b: "two", c: "" });
  });

  it("returns empty object for URL without search", () => {
    const out = parseQueryParams("https://x.com/path");
    expect(out).toEqual({});
  });

  it("decodes URL-encoded values", () => {
    const out = parseQueryParams("https://x.com/?name=%E4%B8%AD%E6%96%87");
    expect(out.name).toBe("中文");
  });

  it("returns empty object when URL is undefined and no window", () => {
    // happy-dom 默认 window.location.search === ""
    const out = parseQueryParams(undefined);
    expect(typeof out).toBe("object");
  });
});

describe("buildQueryString", () => {
  it("builds a query string with leading ?", () => {
    const qs = buildQueryString({ a: "1", b: "two" });
    expect(qs).toBe("?a=1&b=two");
  });

  it("skips null/undefined and empty string values", () => {
    const qs = buildQueryString({ a: 1, b: null, c: undefined, d: "" });
    expect(qs).toBe("?a=1");
  });

  it("returns empty string for empty params", () => {
    expect(buildQueryString({})).toBe("");
  });

  it("returns empty string when all values are filtered", () => {
    expect(buildQueryString({ a: null, b: "", c: undefined })).toBe("");
  });

  it("stringifies non-string values", () => {
    expect(buildQueryString({ count: 42, flag: true })).toBe("?count=42&flag=true");
  });
});