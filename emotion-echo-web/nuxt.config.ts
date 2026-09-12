// nuxt.config.ts
export default defineNuxtConfig({
  router: {
    options: {
      // hashMode: true,
    },
  },
  ssr: false,
  
  runtimeConfig: {
    public: {
      // API 基础地址 = APISIX 网关（决策 11/12：网关是唯一业务入口，
      // 鉴权/限流/熔断/CORS 只在该层做；前端不直连 BFF 或业务 svc）。
      //
      // 2026-09-04 修正：兜底值原为 user-svc 直连 :8888，那是 Stage 26-Q 为绕开
      // APISIX 3.9 的 nginx 301 bug 留的临时方案（注明"留给 Stage 27 升 3.10+ 处理"）。
      // 现镜像已是 3.18.0（决策 11 指定版本），301 不复现；且 Stage 33 PR-20 收紧
      // 端口后 user-svc 不再对宿主暴露，:8888 实测 HTTP 000，该兜底等于没有兜底。
      API_BASE_URL: process.env.NUXT_PUBLIC_API_BASE_URL || "http://localhost:19080/api/v1",
      // 调试配置：是否禁用登录拦截
      DISABLE_AUTH: process.env.NUXT_PUBLIC_DISABLE_AUTH || "false",
      // TTS 服务地址
      ttsBaseUrl: 'http://localhost:8003',
    },
  },
  app: {
    head: {
      // title: "你的项目标题",
      // meta: [{ name: "description", content: "项目描述" }]
    },
  },
  compatibilityDate: "2025-07-15",
  modules: ["@pinia/nuxt", "nuxt-echarts", "@nuxtjs/device"],
  build: {
    transpile: ["naive-ui", "vueuc"],
  },
  devtools: { enabled: true },
  css: ["~/assets/scss/global.scss"],
  components: {
    dirs: [
      {
        path: '~/components',
        extensions: ['.vue'],
        pathPrefix: false
      }
    ]
  },
  routeRules: {
    "/": { redirect: { to: "/chat", statusCode: 301 } },
  },
  vite: {
    optimizeDeps: {
      // Stage 38-A: FaceCamera.vue import '@element-plus/icons-vue' 会让 Vite
      // dev mode 按需 compile 整个包 → FSL 30s。把 icons-vue 加进 include
      // 强制预编译 + 让 CircleClose 等图标首屏可用。
      include: process.env.NODE_ENV === 'development'
        ? ['naive-ui', 'vueuc', 'date-fns-tz/formatInTimeZone', '@element-plus/icons-vue']
        : []
    },
    css: {
      preprocessorOptions: {
        scss: {
          additionalData: `@use "~/assets/scss/variables.scss" as *;`,
        },
      },
    },
    resolve: {
      alias: [
        {
          find: /^dayjs\/plugin\/(.+?)(?:\.js)?$/,
          replacement: 'dayjs/esm/plugin/$1/index.js',
        },
        {
          find: 'dayjs',
          replacement: 'dayjs/esm/index.js',
        },
      ]
    },
    build: {
    },
  },
  echarts: {
    renderer: ["canvas", "svg"],
    charts: ["BarChart", "LineChart", "PieChart", "RadarChart"],
    components: [
      "DatasetComponent",
      "GridComponent",
      "TooltipComponent",
      "LegendComponent",
      "TitleComponent",
      "RadarComponent",
    ],
  },
});