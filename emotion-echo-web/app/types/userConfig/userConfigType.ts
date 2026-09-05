// types/userConfig/userConfigType.ts - 用户配置类型

export type fontSizeType = "14px" | "16px" | "18px" | "20px";
export type themeType = "light" | "dark" | "auto";

/**
 * 用户配置
 */
export interface userConfigType {
  fontSize: fontSizeType;
  theme?: themeType;
}

/**
 * 用户配置请求参数
 */
export interface UpdateUserConfigParams {
  fontSize?: fontSizeType;
  theme?: themeType;
}
