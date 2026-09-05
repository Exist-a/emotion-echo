// utils/unitConvert.js
/**
 * 将vh单位转换为px单位
 * @param {number} vhValue - 要转换的vh/vw数值
 * @returns {number} 转换后的px数值
 */
export const vhToPx = (vhValue:number) => {
  // 服务端环境直接返回0，避免window未定义报错
  if (typeof window === 'undefined') return 0;
  // 核心转换公式：vh值 × 视口高度 / 100
  const viewportHeight = window.innerHeight;
  return Math.round(vhValue * viewportHeight / 100); // 四舍五入避免小数像素
};