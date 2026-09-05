<template>
  <article class="report-scaffold">
    <header class="report-header">
      <div class="header-copy">
        <span class="eyebrow">REFLECTION</span>
        <h2>{{ title }}</h2>
        <p v-if="description" class="description">{{ description }}</p>
      </div>
      <div class="header-control">
        <label v-if="pickerType === 'daterange'" class="date-range">
          <input
            type="date"
            :value="dateStart"
            :max="dateEnd"
            :disabled="loading"
            class="date-input"
            @change="emitRange($event.target.value, dateEnd)"
          />
          <span class="date-sep" aria-hidden="true">至</span>
          <input
            type="date"
            :value="dateEnd"
            :min="dateStart"
            :disabled="loading"
            class="date-input"
            @change="emitRange(dateStart, $event.target.value)"
          />
        </label>
        <input
          v-else
          :type="pickerType === 'month' ? 'month' : pickerType === 'year' ? 'number' : 'date'"
          :value="date"
          :disabled="loading"
          :placeholder="placeholder"
          :min="pickerType === 'year' ? '1970' : undefined"
          :max="maxDate"
          class="date-input"
          @change="emitSingle($event.target.value)"
        />
      </div>
    </header>

    <div v-if="loading" class="loading-state">
      <div class="ee-skeleton" style="height: 22px; width: 60%; margin-bottom: 12px"></div>
      <div class="ee-skeleton" style="height: 80px; margin-bottom: 14px"></div>
      <div class="ee-skeleton" style="height: 240px"></div>
    </div>
    <template v-else>
      <section v-if="$slots.summary" class="report-summary card">
        <slot name="summary" />
      </section>
      <section class="report-charts card">
        <slot name="charts">
          <div class="empty-state">
            <span class="empty-mark">○</span>
            <p>还没有数据</p>
          </div>
        </slot>
      </section>
    </template>
  </article>
</template>

<script setup lang="ts">
type PickerType = 'date' | 'daterange' | 'month' | 'year'

const props = defineProps<{
  title: string
  description?: string
  loading?: boolean
  date: any
  pickerType?: PickerType
  placeholder?: string
  disableFuture?: boolean
}>()

const emit = defineEmits<{
  (e: 'change', value: any): void
  (e: 'update:date', value: any): void
}>()

const placeholder = computed(() => props.placeholder || (props.pickerType === 'daterange' ? '选择一段日期' : '选择一个时间'))

const dateStart = computed(() => Array.isArray(props.date) ? props.date[0] : '')
const dateEnd = computed(() => Array.isArray(props.date) ? props.date[1] : '')

const todayKey = computed(() => {
  const t = new Date()
  const y = t.getFullYear()
  const m = String(t.getMonth() + 1).padStart(2, '0')
  const d = String(t.getDate()).padStart(2, '0')
  return { y, m, d, yearMonth: `${y}-${m}`, iso: `${y}-${m}-${d}` }
})

// disableFuture 应用到 native input 的 max 属性
const maxDate = computed(() => {
  if (!props.disableFuture) return undefined
  const k = todayKey.value
  if (props.pickerType === 'year') return String(k.y)
  if (props.pickerType === 'month') return k.yearMonth
  return k.iso
})

const emitSingle = (value: string) => {
  emit('change', value)
  emit('update:date', value)
}
const emitRange = (start: string, end: string) => {
  const next = start && end ? [start, end] : []
  emit('change', next)
  emit('update:date', next)
}
</script>

<style scoped lang="scss">
.report-scaffold { display: grid; gap: 18px; width: min(1080px, 100%); margin: 0 auto; }
.report-header { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 18px; }
.header-copy { display: grid; gap: 6px; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: 0.16em; }
.header-copy h2 { margin: 0; font-size: clamp(22px, 2.5vw, 28px); font-weight: 600; letter-spacing: -0.02em; }
.description { margin: 0; color: var(--ee-text-muted); font-size: 14px; }

.header-control .date-range { display: inline-flex; align-items: center; gap: 6px; }
.header-control .date-input {
  min-width: 150px;
  height: 36px;
  padding: 0 10px;
  color: var(--ee-text);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font: inherit;
  outline: none;
  transition: border-color var(--ee-transition);
}
.header-control .date-input:focus { border-color: var(--ee-primary); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ee-primary) 25%, transparent); }
.header-control .date-sep { color: var(--ee-text-muted); font-size: 12px; }

.card { background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); padding: 18px 20px; }
.report-summary { display: grid; gap: 16px; }
.report-charts { min-height: 240px; }

.loading-state { display: grid; gap: 12px; }
.ee-skeleton { background: linear-gradient(90deg, var(--ee-surface-muted) 25%, color-mix(in srgb, var(--ee-surface-muted) 50%, var(--ee-surface)) 50%, var(--ee-surface-muted) 75%); background-size: 200% 100%; animation: skeleton-shimmer 1.4s ease-in-out infinite; border-radius: var(--ee-radius-sm, 6px); }
@keyframes skeleton-shimmer { from { background-position: 200% 0; } to { background-position: -200% 0; } }

.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 40px 8px; color: var(--ee-text-muted); }
.empty-mark { font-size: 28px; opacity: 0.6; }
.empty-state p { margin: 0; font-size: 13px; }

@media (max-width: 600px) {
  .report-header { align-items: stretch; flex-direction: column; }
  .header-control .date-input { width: 100%; min-width: 0; }
  .header-control .date-range { flex-wrap: wrap; }
}
</style>
