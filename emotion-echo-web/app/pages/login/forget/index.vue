<template>
  <section class="forget-page">
    <ol class="forget-steps" aria-label="找回密码步骤">
      <li v-for="(step, index) in steps" :key="step.label" :class="{ 'is-active': index === active, 'is-done': index < active }">
        <span class="dot">{{ index + 1 }}</span>
        <span class="label">{{ step.label }}</span>
      </li>
    </ol>
    <main class="forget-main">
      <NuxtPage @changeActive="changeActive" />
    </main>
  </section>
</template>

<script setup lang="ts">
const route = useRoute()
const router = useRouter()

const steps = [
  { label: '确认账户', path: '/login/forget/verify' },
  { label: '修改密码', path: '/login/forget/modify' },
  { label: '修改成功', path: '/login/forget/success' }
]

const active = ref(Math.max(0, steps.findIndex((step) => step.path === route.path)))

watch(() => route.path, (path) => {
  active.value = Math.max(0, steps.findIndex((step) => step.path === path))
}, { immediate: true })

const changeActive = () => {
  const next = steps[active.value + 1]
  if (next) router.push(next.path)
}
</script>

<style scoped lang="scss">
.forget-page { width: min(720px, 100%); margin: 0 auto; padding: clamp(28px, 5vw, 56px) clamp(16px, 4vw, 24px); }
.forget-steps { display: flex; gap: 16px; padding: 0; margin-bottom: 28px; }
.forget-steps li { display: flex; align-items: center; gap: 8px; padding: 8px 12px; color: var(--ee-text-muted); background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: 999px; flex: 1; }
.forget-steps li .dot { display: grid; width: 22px; height: 22px; place-items: center; background: var(--ee-surface-muted); border-radius: 50%; font-size: 12px; font-weight: 600; }
.forget-steps li.is-active { color: var(--ee-primary); border-color: var(--ee-primary); background: var(--ee-primary-soft); }
.forget-steps li.is-active .dot { background: var(--ee-primary); color: #fff; }
.forget-steps li.is-done .dot { color: #fff; background: var(--ee-primary); }
.forget-steps li .label { font-size: 13px; }
.forget-main { background: transparent; }
@media (max-width: 600px) { .forget-steps { flex-direction: column; } }
</style>
