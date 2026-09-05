<template>
  <section class="setting-page">
    <header class="page-intro">
      <span class="eyebrow">PERSONALIZE</span>
      <h2>让这里更像你的空间</h2>
      <p>调整阅读体验，找到最舒服的节奏。</p>
    </header>

    <div class="settings-list">
      <div class="setting-item font-size-edit">
        <div class="setting-copy">
          <span class="setting-title">字体大小</span>
          <span class="setting-description">消息文字的阅读尺寸</span>
        </div>
        <div class="setting-control">
          <span class="preview" :style="{ fontSize: fontSizeValue }">Aa</span>
          <div class="font-size-menu">
            <button
              v-for="opt in fontSizeOptions"
              :key="opt.value"
              type="button"
              class="font-size-btn"
              :class="{ active: userConfig.fontSize === opt.value }"
              @click="handleFontSizeChange(opt.value)"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>
      </div>

      <div class="setting-item theme-edit">
        <div class="setting-copy">
          <span class="setting-title">界面主题</span>
          <span class="setting-description">选择你喜欢的明暗方式</span>
        </div>
        <div class="theme-options">
          <label v-for="opt in themeOptions" :key="opt.value" class="theme-option" :class="{ active: userConfig.theme === opt.value }">
            <input
              type="radio"
              name="theme"
              :value="opt.value"
              :checked="userConfig.theme === opt.value"
              @change="handleThemeChange(opt.value)"
            />
            <span>{{ opt.label }}</span>
          </label>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { themeType } from '~/types/userConfig/userConfigType'

const userStore = useUserStore()
const userConfig = ref(userStore.getUserConfig())

const fontSizes = { small: '14px', medium: '16px', large: '18px' } as const
const fontLabels = { small: '小', medium: '中', large: '大' } as const
const fontSizeOptions = [
  { value: 'small' as const, label: '小' },
  { value: 'medium' as const, label: '中' },
  { value: 'large' as const, label: '大' }
]
const themeOptions: { value: 'light' | 'dark' | 'auto', label: string }[] = [
  { value: 'light', label: '浅色' },
  { value: 'dark', label: '深色' },
  { value: 'auto', label: '跟随系统' }
]

const fontSizeValue = computed(() => fontSizes[userConfig.value.fontSize as keyof typeof fontSizes] || '16px')
const fontSizeLabel = computed(() => fontLabels[userConfig.value.fontSize as keyof typeof fontLabels] || '中')

const handleFontSizeChange = async (size: 'small' | 'medium' | 'large') => {
  userConfig.value.fontSize = size
  await userStore.setFontSize(size)
}

const handleThemeChange = async (theme: string | number | boolean | undefined) => {
  const t = (theme as themeType) || 'auto'
  userConfig.value.theme = t
  await userStore.setTheme(t)
}
</script>

<style scoped lang="scss">
.setting-page { width: min(760px, 100%); margin: 0 auto; }
.page-intro { margin-bottom: 28px; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: 0.16em; }
.page-intro h2 { margin: 6px 0 0; font-size: clamp(22px, 2.6vw, 28px); font-weight: 600; letter-spacing: -0.02em; }
.page-intro p { margin: 6px 0 0; color: var(--ee-text-muted); font-size: 14px; }

.settings-list { display: grid; gap: 12px; }
.setting-item { display: flex; align-items: center; justify-content: space-between; gap: 22px; min-height: 86px; padding: 18px 20px; background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); transition: border-color var(--ee-transition), transform var(--ee-transition); }
.setting-item:hover { border-color: color-mix(in srgb, var(--ee-primary) 50%, var(--ee-border)); transform: translateY(-1px); }
.setting-copy { display: grid; gap: 3px; }
.setting-title { font-size: 15px; font-weight: 600; }
.setting-description { color: var(--ee-text-muted); font-size: 12px; }

.setting-control { display: flex; align-items: center; gap: 14px; }
.preview { display: grid; width: 40px; height: 40px; place-items: center; color: var(--ee-primary); background: var(--ee-primary-soft); border-radius: 10px; font-weight: 600; }

.font-size-menu { display: flex; gap: 4px; padding: 3px; background: var(--ee-surface-muted); border-radius: 10px; }
.font-size-btn { min-width: 38px; height: 30px; padding: 0 12px; color: var(--ee-text-muted); background: transparent; border: 0; border-radius: 7px; cursor: pointer; font-size: 13px; font-weight: 600; transition: color var(--ee-transition), background var(--ee-transition); }
.font-size-btn:hover { color: var(--ee-text); }
.font-size-btn.active { color: var(--ee-text); background: var(--ee-surface); box-shadow: 0 1px 2px rgba(32, 37, 34, 0.06); }

.theme-options { display: flex; gap: 4px; padding: 3px; background: var(--ee-surface-muted); border-radius: 10px; }
.theme-option { display: inline-flex; align-items: center; gap: 6px; min-width: 70px; height: 30px; padding: 0 12px; color: var(--ee-text-muted); border-radius: 7px; cursor: pointer; font-size: 13px; font-weight: 600; transition: color var(--ee-transition), background var(--ee-transition); }
.theme-option:hover { color: var(--ee-text); }
.theme-option.active { color: var(--ee-text); background: var(--ee-surface); box-shadow: 0 1px 2px rgba(32, 37, 34, 0.06); }
.theme-option input { position: absolute; width: 1px; height: 1px; opacity: 0; pointer-events: none; }

@media (max-width: 600px) {
  .setting-item { align-items: flex-start; flex-direction: column; }
  .setting-control { width: 100%; justify-content: flex-end; }
  .theme-options { flex-wrap: wrap; }
}
</style>
