<template>
  <div class="app-shell">
    <aside class="app-sidebar" :class="{ 'is-open': isSidebarOpen }">
      <div class="brand-block">
        <NuxtLink to="/chat/conversation" class="brand" @click="closeSidebar">
          <span class="brand-name">情绪回音</span>
        </NuxtLink>
        <button class="icon-button mobile-only" type="button" aria-label="关闭导航" @click="closeSidebar">×</button>
      </div>

      <nav class="nav-list" aria-label="主导航">
        <NuxtLink v-for="item in primaryLinks" :key="item.to" :to="item.to" class="nav-link" :class="{ active: isActive(item.to) }" @click="closeSidebar">
          <span class="nav-icon" aria-hidden="true">{{ item.icon }}</span>
          <span>{{ item.label }}</span>
        </NuxtLink>
      </nav>

      <div class="nav-section-label">回顾与了解</div>
      <nav class="nav-list" aria-label="辅助导航">
        <NuxtLink v-for="item in secondaryLinks" :key="item.to" :to="item.to" class="nav-link" :class="{ active: isActive(item.to) }" @click="closeSidebar">
          <span class="nav-icon" aria-hidden="true">{{ item.icon }}</span>
          <span>{{ item.label }}</span>
        </NuxtLink>
      </nav>

      <div class="sidebar-footer">
        <span class="status-dot" aria-hidden="true"></span>
        <span>此刻，我在听</span>
      </div>
    </aside>

    <div v-if="isSidebarOpen" class="sidebar-backdrop mobile-only" aria-hidden="true" @click="closeSidebar"></div>
    <main class="app-content">
      <header class="app-header">
        <button class="icon-button mobile-only" type="button" aria-label="打开导航" @click="openSidebar">☰</button>
        <div class="header-context">
          <span class="eyebrow">{{ pageEyebrow }}</span>
          <h1>{{ pageTitle }}</h1>
        </div>
        <div class="header-actions">
          <NuxtLink to="/chat/setting" class="avatar-link" aria-label="打开设置">
            <span class="avatar-initial">{{ userInitial }}</span>
          </NuxtLink>
        </div>
      </header>
      <div class="page-content"><slot /></div>
    </main>
  </div>
</template>

<script setup lang="ts">
const route = useRoute()
const isSidebarOpen = ref(false)

const primaryLinks = [
  { to: '/chat/conversation', label: '对话', icon: '○' },
  { to: '/question', label: '心理测验', icon: '✦' }
]

const secondaryLinks = [
  { to: '/chat/dashboard/dailyReport', label: '日报', icon: '◒' },
  { to: '/chat/dashboard/weeklyReport', label: '周报', icon: '◒' },
  { to: '/chat/dashboard/monthlyReport', label: '月报', icon: '◒' },
  { to: '/chat/dashboard/annualReport', label: '年报', icon: '◒' },
  { to: '/chat/user', label: '我的空间', icon: '⌂' },
  { to: '/chat/setting', label: '设置', icon: '⌘' }
]

const isActive = (path: string) => {
  if (path === '/chat/conversation') return route.path.startsWith('/chat/conversation')
  if (path.startsWith('/chat/dashboard/')) return route.path === path
  return route.path === path || route.path.startsWith(`${path}/`)
}

const pageMeta = computed(() => {
  const match = [...primaryLinks, ...secondaryLinks].find((item) => isActive(item.to))
  return match || primaryLinks[0]
})
const pageTitle = computed(() => pageMeta.value?.label || 'Emotion Echo')
const pageEyebrow = computed(() => route.path.startsWith('/chat/conversation') ? 'CONVERSATION' : route.path.startsWith('/chat/dashboard') ? 'REFLECTION' : 'EMOTION ECHO')

const userStore = useUserStore()
const userInitial = computed(() => {
  const name = userStore.getNickname || userStore.userInfo?.username || '我'
  return String(name).slice(0, 1).toUpperCase()
})

const closeSidebar = () => { isSidebarOpen.value = false }
const openSidebar = () => { isSidebarOpen.value = true }
watch(() => route.path, closeSidebar)
</script>

<style scoped lang="scss">
.app-shell {
  display: flex;
  min-height: 100vh;
  background: var(--ee-bg);
}

.app-sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  z-index: 20;
  display: flex;
  width: 248px;
  flex-direction: column;
  padding: 28px 18px 22px;
  background: var(--ee-surface);
  border-right: 1px solid var(--ee-border);
}

.brand-block,
.app-header,
.header-actions,
.brand,
.nav-link,
.sidebar-footer {
  display: flex;
  align-items: center;
}

.brand-block {
  justify-content: space-between;
  margin: 0 10px 42px;
}

.brand { padding: 6px 12px; }
.brand-name { color: var(--ee-text); font-size: 16px; font-weight: 600; letter-spacing: -0.02em; }

.nav-section-label {
  margin: 28px 12px 9px;
  color: var(--ee-text-muted);
  font-size: 11px;
  letter-spacing: 0.12em;
}
.nav-list { display: grid; gap: 5px; }
.nav-link {
  gap: 12px;
  min-height: 44px;
  padding: 0 12px;
  color: var(--ee-text-muted);
  border-radius: var(--ee-radius-md);
  font-size: 14px;
  transition: color var(--ee-transition), background var(--ee-transition), transform var(--ee-transition);
}
.nav-link:hover { color: var(--ee-text); background: var(--ee-surface-muted); transform: translateX(2px); }
.nav-link.active { color: var(--ee-primary); background: var(--ee-primary-soft); font-weight: 600; }
.nav-icon { width: 20px; color: currentColor; text-align: center; font-size: 17px; }
.sidebar-footer { gap: 8px; margin: auto 12px 0; color: var(--ee-text-muted); font-size: 12px; }
.status-dot { width: 7px; height: 7px; background: var(--ee-primary); border-radius: 50%; box-shadow: 0 0 0 4px var(--ee-primary-soft); }

.app-content { width: calc(100% - 248px); min-height: 100vh; margin-left: 248px; }
.app-header { min-height: 88px; justify-content: space-between; padding: 22px clamp(24px, 4vw, 64px) 12px; }
.header-context h1 { margin-top: 2px; font-size: clamp(20px, 2vw, 26px); font-weight: 600; letter-spacing: -0.03em; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: 0.16em; }
.avatar-link { display: grid; width: 36px; height: 36px; place-items: center; background: var(--ee-primary-soft); border-radius: 50%; }
.avatar-initial { color: var(--ee-primary); font-size: 13px; font-weight: 700; }
.page-content { min-height: calc(100vh - 88px); padding: 12px clamp(24px, 4vw, 64px) 40px; }
.icon-button { display: grid; width: 36px; height: 36px; place-items: center; color: var(--ee-text-muted); background: transparent; border: 1px solid var(--ee-border); border-radius: var(--ee-radius-md); }
.icon-button:hover { color: var(--ee-primary); border-color: var(--ee-primary); }
.mobile-only { display: none; }

@media (max-width: 768px) {
  .mobile-only { display: grid; }
  .app-sidebar { width: min(82vw, 300px); transform: translateX(-100%); transition: transform var(--ee-transition); box-shadow: var(--ee-shadow-soft); }
  .app-sidebar.is-open { transform: translateX(0); }
  .sidebar-backdrop { position: fixed; inset: 0; z-index: 19; background: rgba(20, 27, 23, 0.38); backdrop-filter: blur(2px); }
  .app-content { width: 100%; margin-left: 0; }
  .app-header { gap: 12px; min-height: 78px; padding: 16px 18px 8px; }
  .app-header .mobile-only { flex-shrink: 0; }
  .header-context { flex: 1; }
  .page-content { min-height: calc(100vh - 78px); padding: 10px 14px 28px; }
}
</style>
