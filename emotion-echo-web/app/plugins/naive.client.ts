// plugins/naive.client.ts
// 在客户端挂载 Naive UI 的 message / dialog / notification provider，
// 并提供与原 ElNotification 兼容的全局工具函数。
import {
  create,
  NConfigProvider,
  NMessageProvider,
  NDialogProvider,
  NNotificationProvider,
  NLoadingBarProvider,
  NButton,
  NInput,
  NIcon,
  NCard,
  NCheckbox,
  NForm,
  NFormItem,
  NSpace,
  NSkeleton,
  NEmpty,
  NDivider,
  NAvatar,
  NTooltip,
  NPopover,
  NDropdown,
  NSpin
} from 'naive-ui'

const naive = create({
  components: [
    NConfigProvider, NMessageProvider, NDialogProvider, NNotificationProvider, NLoadingBarProvider,
    NButton, NInput, NIcon, NCard, NCheckbox, NForm, NFormItem, NSpace, NSkeleton, NEmpty,
    NDivider, NAvatar, NTooltip, NPopover, NDropdown, NSpin
  ]
})

export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.vueApp.use(naive)
})
