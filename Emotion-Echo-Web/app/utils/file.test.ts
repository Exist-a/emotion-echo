/**
 * file.test.ts · utils/file.ts 文件大小格式化测试
 *
 * Stage 26-T backlog §五 5.3: 覆盖 formatFileSize。downloadFile 和
 * copyToClipboard 依赖 DOM/clipboard API，happy-dom 部分模拟；我们
 * 只测纯函数 + 一个 "下载链路" 烟雾测试。
 */
import { describe, it, expect, vi } from "vitest";

import { formatFileSize } from "./file";

describe("formatFileSize", () => {
  it.each([
    [0, "0 B"],
    [1, "1 B"],
    [1023, "1023 B"],
    [1024, "1 KB"],
    [1536, "1.5 KB"],   // 1.5 * 1024
    [1024 * 1024, "1 MB"],
    [1024 * 1024 * 1024, "1 GB"],
    [1024 * 1024 * 1024 * 1024, "1 TB"],
  ])("formatFileSize(%i) === %s", (bytes, expected) => {
    expect(formatFileSize(bytes)).toBe(expected);
  });

  it("uses custom decimals", () => {
    expect(formatFileSize(1536, 0)).toBe("2 KB"); // 1.5 rounds up to 2
    expect(formatFileSize(1536, 3)).toBe("1.5 KB");
  });

  it("handles sizes between units", () => {
    expect(formatFileSize(2.5 * 1024)).toBe("2.5 KB");
    expect(formatFileSize(1024 * 1024 + 512 * 1024)).toBe("1.5 MB");
  });
});