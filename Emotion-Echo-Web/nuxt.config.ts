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
      // API 基础地址
      API_BASE_URL: process.env.NUXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1",
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
      include: process.env.NODE_ENV === 'development'
        ? ['naive-ui', 'vueuc', 'date-fns-tz/formatInTimeZone']
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
      rollupOptions: {
        output: {
          manualChunks(id) {
            // 将 echarts 相关模块合并为单个 chunk，减少请求数
            if (id.includes('echarts')) {
              return 'echarts';
            }
            // 将 element-plus 合并为单个 chunk
            if (id.includes('element-plus')) {
              return 'element-plus';
            }
            // 将 node_modules 中的其他大型依赖合并为 vendor chunk
            if (id.includes('node_modules')) {
              return 'vendor';
            }
          },
        },
      },
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