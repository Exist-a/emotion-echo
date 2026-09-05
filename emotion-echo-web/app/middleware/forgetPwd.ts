// middleware/forgetPwd.ts
import type { RouteLocationNormalized } from "vue-router";
import { useForgetPwdState } from "~/composables/forgetPwdState";

// 定义「目标路由-所需最小步骤」的映射（仅适配两个页面）
const stepRouteMap = {
  "/login/forget/modify": 1, // 修改密码页：需 step ≥ 1
  "/login/forget/success": 2, // 成功页：需 step ≥ 2
};

// 核心中间件：适配两个页面
export default defineNuxtRouteMiddleware(
  (to: RouteLocationNormalized, from: RouteLocationNormalized) => {
    // 1. 只处理目标路由，其他路由直接放行（避免影响其他页面）
    const targetPaths = Object.keys(stepRouteMap);
    if (!targetPaths.includes(to.path)) {
      return;
    }

    // 2. 获取当前路由所需的最小步骤
    const currentPath = to.path.replace(/\/$/, "");

    const requiredStep = stepRouteMap[currentPath as keyof typeof stepRouteMap];

    const { currentStep } = useForgetPwdState();

    // 3. 核心校验：步骤不足则跳回确认账号页（或你指定的初始页）
    if (currentStep.value < requiredStep) {
      return navigateTo("/login/forget/verify"); // 跳回流程初始页
    }

    // 4. 可选：from 辅助校验（按页面定制，仅日志提醒，不影响核心逻辑）
    switch (to.path) {
      case "/login/forget/modify":
        // 修改密码页：只允许从验证码验证页跳转（刷新页面时 from.path 为当前页，需排除）
        if (from.path !== "/login/forget/verify" && from.path !== to.path) {
          console.warn("【修改密码页】用户从非验证页进入：", from.path);
        }
        break;
      case "/login/forget/success":
        // 成功页：只允许从修改密码页跳转
        if (from.path !== "/login/forget/modify" && from.path !== to.path) {
          console.warn("【成功页】用户从非修改密码页进入：", from.path);
        }
        break;
    }
  }
);
