/**
 * 安全访问工具函数
 */

export function safeGet(
  obj: any,
  path: string,
  defaultValue?: any
): any {
  const keys = path.split(".");
  let result = obj;

  for (const key of keys) {
    if (result == null || typeof result !== "object") {
      return defaultValue;
    }
    result = result[key];
  }

  return result !== undefined ? result : defaultValue;
}
