// app/router.options.ts
// @ts-nocheck
import type { RouterConfig } from '@nuxt/schema'

export default {
  // 自定义路由表（覆盖 Nuxt 自动生成的路由）
  routes: (_routes) => [
    // 根路由：重定向到 /chat（核心需求）
    {
      name: 'root',
      path: '/',
      redirect: { name: 'chat' }, // 直接重定向到聊天页
    },
    // 问题页
    {
      name: 'question',
      path: '/question',
      component: () => import('~/pages/question/index.vue'),
    },
    {
      name: 'question-detail',
      path: '/question/:id',
      component: () => import('~/pages/question/[id].vue'),
    },
    // 聊天页（需要登录校验）
    {
      name: 'chat',
      path: '/chat',
      redirect: '/chat/conversation/new',
      component: () => import('~/pages/chat/index.vue'),
      children: [
        {
          name: 'chat-conversation',
          path: 'conversation',
          redirect: '/chat/conversation/new',

          component: () => import('~/pages/chat/conversation/index.vue'),
          children: [
            {
              name: 'chat-conversation-detail',
              path: ':id',
              component: () => import('~/pages/chat/conversation/[id].vue'),
              props: true,
            },
            {
              name: 'chat-conversation-new',
              path: 'new',
              component: () => import('~/pages/chat/conversation/new.vue'),
            },
          ],
        },
        {
          name: 'chat-user',
          path: 'user',
          component: () => import('~/pages/chat/user/index.vue'),
        },
        {
          name: 'chat-setting',
          path: 'setting',
          component: () => import('~/pages/chat/setting.vue'),
        },
        // CL-06：角色管理（用户自定义 Agent）。
        // ⚠️ **必须在这里登记** —— 本文件是**手写路由表**（`routes: (_routes) => [...]`
        // 忽略了自动生成的路由），所以"文件系统即路由"在本仓**不成立**：
        // 只建 `app/pages/chat/agents.vue` 不会产生任何路由，访问会 404。
        // 这条陷阱由 CL-06 Phase 8 生产构建实测抓出（本地 build 同样可复现），
        // 回归钉见 `app/router.options.contract.test.ts`（会扫 pages/ 全量比对）。
        {
          name: 'chat-agents',
          path: 'agents',
          component: () => import('~/pages/chat/agents.vue'),
        },
        // CL-08a：「我的记忆」自述入口 + 记忆图（同样必须在此登记，否则 404）
        {
          name: 'chat-memory',
          path: 'memory',
          component: () => import('~/pages/chat/memory.vue'),
        },
        {
          name: 'chat-dashboard',
          path: 'dashboard',
          redirect: '/chat/dashboard/dailyReport',
          component: () => import('~/pages/chat/dashboard/index.vue'),

          children: [
            {
              name: 'chat-dashboard-dailyReport',
              path: 'dailyReport',
              component: () => import('~/pages/chat/dashboard/dailyReport.vue'),
            },
            {
              name: 'chat-dashboard-weeklyReport',
              path: 'weeklyReport',
              component: () => import('~/pages/chat/dashboard/weeklyReport.vue'),
            },
            {
              name: 'chat-dashboard-monthlyReport',
              path: 'monthlyReport',
              component: () => import('~/pages/chat/dashboard/monthlyReport.vue'),
            },
            {
              name: 'chat-dashboard-annualReport',
              path: 'annualReport',
              component: () => import('~/pages/chat/dashboard/annualReport.vue'),
            },
            //....
          ],
        },
      ],
      meta: { requiresAuth: true, layout: 'nav' }, // 标记需要登录
    },

    // 登录页
    {
      name: 'login',
      path: '/login',
      component: () => import('~/pages/login/index.vue'),
    },

    // 忘记密码（父路由）- 嵌套子路由
    {
      name: 'login-forget',
      path: '/login/forget',
      component: () => import('~/pages/login/forget/index.vue'),
      children: [
        {
          name: 'login-forget-verify',
          path: 'verify',
          component: () => import('~/pages/login/forget/verify.vue'),
        },
        {
          name: 'login-forget-modify',
          path: 'modify',
          meta: {
            middleware: 'forget-pwd',
          },
          component: () => import('~/pages/login/forget/modify.vue'),
        },
        {
          name: 'login-forget/success',
          path: 'success',
          meta: {
            middleware: 'forget-pwd',
          },
          component: () => import('~/pages/login/forget/success.vue'),
        },
      ],
    },
  ],

  // 可选：Vue Router 全局配置
  // strict: true, // Vue Router 4 已废弃此选项
  sensitive: false, // 路径大小写不敏感
} satisfies RouterConfig
