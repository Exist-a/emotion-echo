// types/commonType.ts - 通用类型定义

/**
 * 通用返回类型
 */
export interface returnMsgType {
  isOk: boolean;
  msg: string;
  data?: any;
}

/**
 * 异步操作结果
 */
export interface AsyncResult<T = any> {
  success: boolean;
  data?: T;
  error?: string;
}

/**
 * 键值对对象
 */
export interface KeyValue<T = any> {
  [key: string]: T;
}

/**
 * 可选部分
 */
export type Optional<T, K extends keyof T> = Omit<T, K> & Partial<Pick<T, K>>;

/**
 * 必填部分（将指定字段设为必填）
 */
export type MakeRequired<T, K extends keyof T> = T & Required<Pick<T, K>>;

/**
 * 可空类型
 */
export type Nullable<T> = T | null | undefined;
