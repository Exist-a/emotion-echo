// utils/Regs.ts - 常用正则表达式

/**
 * 密码正则
 * 要求：6-18位，包含字母和数字
 */
export const passwordReg = /^(?=.*[a-zA-Z])(?=.*\d).{6,18}$/;

/**
 * 手机号正则
 */
export const phoneReg = /^1[3-9]\d{9}$/;

/**
 * 邮箱正则
 */
export const emailReg = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;

/**
 * 手机号或邮箱正则
 */
export const phoneOrEmailReg = /^(?:1[3-9]\d{9}|[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})$/;

/**
 * 验证码正则（6位数字）
 */
export const verificationCodeReg = /^\d{6}$/;

/**
 * 昵称正则
 * 允许：中英文（含繁体）、数字（全角/半角）、下划线、横线、中文间隔号 ·
 * 长度：2-12个字符
 */
export const nicknameReg = /^[a-zA-Z0-9_\u4e00-\u9fa5\u3400-\u4dbf\u{20000}-\u{2a6df}\u{2a700}-\u{2b73f}\u{2b740}-\u{2b81f}\u{2b820}-\u{2ceaf}\u{2ceb0}-\u{2ebef}\u{30000}-\u{3134f}\uff00-\uffef·-]{2,12}$/u;

/**
 * URL 正则
 */
export const urlReg = /^(https?:\/\/)?([\da-z.-]+)\.([a-z.]{2,6})([/\w .-]*)*\/?$/;

/**
 * 身份证号正则（简化版）
 */
export const idCardReg = /^\d{15}|\d{18}$/;

/**
 * Emoji 表情正则
 */
export const emojiReg = /[\u{1F600}-\u{1F64F}]|[\u{1F300}-\u{1F5FF}]|[\u{1F680}-\u{1F6FF}]|[\u{1F1E0}-\u{1F1FF}]|[\u{2600}-\u{26FF}]|[\u{2700}-\u{27BF}]/gu;

// ==================== 验证函数 ====================

/**
 * 验证密码
 * @param password 密码
 */
export function validatePassword(password: string): boolean {
  return passwordReg.test(password);
}

/**
 * 验证手机号
 * @param phone 手机号
 */
export function validatePhone(phone: string): boolean {
  return phoneReg.test(phone);
}

/**
 * 验证邮箱
 * @param email 邮箱
 */
export function validateEmail(email: string): boolean {
  return emailReg.test(email);
}

/**
 * 验证手机号或邮箱
 * @param value 值
 */
export function validatePhoneOrEmail(value: string): boolean {
  return phoneOrEmailReg.test(value);
}

/**
 * 验证验证码
 * @param code 验证码
 */
export function validateVerificationCode(code: string): boolean {
  return verificationCodeReg.test(code);
}

/**
 * 验证昵称
 * @param nickname 昵称
 */
export function validateNickname(nickname: string): boolean {
  return nicknameReg.test(nickname);
}

/**
 * 验证 URL
 * @param url URL
 */
export function validateUrl(url: string): boolean {
  return urlReg.test(url);
}

/**
 * 获取验证错误信息
 * @param type 验证类型
 */
export function getValidationError(type: "password" | "phone" | "email" | "nickname" | "code"): string {
  const errorMap: Record<string, string> = {
    password: "密码需为6-18位，且包含字母和数字",
    phone: "请输入有效的手机号",
    email: "请输入有效的邮箱地址",
    nickname: "昵称长度为2-12个字符，支持中英文、数字、下划线",
    code: "验证码为6位数字",
  };
  return errorMap[type] || "格式错误";
}
