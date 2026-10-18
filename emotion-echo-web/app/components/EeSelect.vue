<template>
  <div ref="rootRef" class="ee-select">
    <button
      ref="triggerRef"
      type="button"
      class="ee-select-trigger"
      :aria-expanded="open ? 'true' : 'false'"
      aria-haspopup="listbox"
      :aria-label="ariaLabel"
      :data-testid="testid"
      @click="toggle"
      @keydown="onTriggerKeydown"
    >
      <span class="ee-select-value" :class="{ 'is-placeholder': !matched }">{{ currentLabel }}</span>
      <span class="ee-select-caret" aria-hidden="true">▾</span>
    </button>

    <!-- 面板 Teleport 到 body：
         否则它会被最近的滚动容器裁掉（实测：放进弹窗里时，面板在弹窗中段就被切断了，
         只剩一个分组标题露出来）。Teleport + fixed 定位让它永远完整可见，
         并能在下方空间不足时翻到上方展开。 -->
    <Teleport to="body">
      <div
        v-if="open"
        class="ee-select-panel"
        :style="panelStyle"
        role="listbox"
        tabindex="-1"
        :aria-label="ariaLabel"
        @keydown="onPanelKeydown"
      >
        <template v-for="g in groups" :key="g.label">
          <p v-if="g.label" class="ee-select-group-label">{{ g.label }}</p>
          <button
            v-for="o in g.items"
            :key="o.value"
            type="button"
            class="ee-select-option"
            role="option"
            :class="{ 'is-active': o.index === activeIndex, 'is-selected': o.value === modelValue }"
            :aria-selected="o.value === modelValue ? 'true' : 'false'"
            :data-ee-option="o.value"
            @click="choose(o.value)"
            @mousemove="activeIndex = o.index"
          >
            {{ o.label }}
          </button>
        </template>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
// CL-06 · UI 轮：通用**自绘下拉**。
//
// 存在的唯一理由：原生 <select> 展开的那一层由浏览器/操作系统绘制，不在 DOM 里，
// CSS 完全够不着 ⇒ 主题色、深色模式、分组统统做不到（用户实测反馈）。
// 自绘之后，这些"原生白送的行为"必须自己实现，本组件把它们当成契约：
//   展开 / 选中 / Esc 取消 / 点外面关闭 / 方向键 + Enter / 不被裁切。
//
// 最后一条（不被裁切）靠 Teleport + fixed 定位兜住：只要面板还留在原地，
// 它就会被最近的 overflow 容器切掉 —— 弹窗里必然踩到。
//
// 纯受控组件：不持有选中值，只在选择时 emit。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { computeDropdownLayout } from '~/utils/dropdownLayout'

export interface EeSelectOption {
  value: string
  label: string
  /** 同组选项会渲染在一个标题下（音色按女声/男声分组） */
  group?: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: EeSelectOption[]
    /** 透传到触发器（页面契约靠它定位控件） */
    testid?: string
    ariaLabel?: string
    /** 当前值不在选项里时显示什么（老数据/选项下线） */
    placeholder?: string
  }>(),
  { testid: undefined, ariaLabel: undefined, placeholder: '请选择' },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

/**
 * 按触发器的位置算面板坐标（fixed）；往下放不下就翻到上方。
 * 判断与算术都在 `~/utils/dropdownLayout` 的纯函数里（有单测钉边界）。
 */
const place = () => {
  if (typeof window === 'undefined') return
  const rect = triggerRef.value?.getBoundingClientRect()
  if (!rect) return
  const layout = computeDropdownLayout(
    { top: rect.top, bottom: rect.bottom },
    window.innerHeight,
  )
  panelStyle.value = {
    position: 'fixed',
    left: `${Math.round(rect.left)}px`,
    width: `${Math.round(rect.width)}px`,
    maxHeight: `${Math.round(layout.maxHeight)}px`,
    top: layout.top === 'auto' ? 'auto' : `${Math.round(layout.top)}px`,
    bottom: layout.bottom === 'auto' ? 'auto' : `${Math.round(layout.bottom)}px`,
  }
}

const rootRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const open = ref(false)
const panelStyle = ref<Record<string, string>>({})

/** 拍平并记住原始下标：键盘高亮用下标在**拍平后**的顺序上移动 */
const flat = computed(() => props.options.map((o, index) => ({ ...o, index })))

const groups = computed(() => {
  const out: { label: string; items: typeof flat.value }[] = []
  for (const o of flat.value) {
    const label = o.group ?? ''
    const last = out[out.length - 1]
    if (last && last.label === label) last.items.push(o)
    else out.push({ label, items: [o] })
  }
  return out
})

const matched = computed(() => props.options.find((o) => o.value === props.modelValue) ?? null)
const currentLabel = computed(() => matched.value?.label ?? props.placeholder)
const activeIndex = ref(0)

/**
 * 点外面关闭。
 *
 * 注意：**触发器本身的点击也会冒泡到 document** —— 触发器在 rootRef 内部，命中即忽略，
 * 所以不依赖"watch 是否赶在事件冒泡之前跑完"这种时序巧合。
 */
const onDocumentClick = (e: MouseEvent) => {
  const target = e.target as Node
  const panel = document.querySelector('.ee-select-panel')
  if (!rootRef.value?.contains(target) && !panel?.contains(target)) open.value = false
}

watch(open, (isOpen) => {
  if (typeof window === 'undefined') return
  if (isOpen) {
    document.addEventListener('click', onDocumentClick)
    window.addEventListener('resize', place)
    // capture=true：内层滚动容器（弹窗主体、消息列表）滚动时也要重新定位
    window.addEventListener('scroll', place, true)
    place()
  } else {
    document.removeEventListener('click', onDocumentClick)
    window.removeEventListener('resize', place)
    window.removeEventListener('scroll', place, true)
  }
})

onBeforeUnmount(() => {
  if (typeof window === 'undefined') return
  document.removeEventListener('click', onDocumentClick)
  window.removeEventListener('resize', place)
  window.removeEventListener('scroll', place, true)
})

const openWithActive = (delta = 0) => {
  const current = matched.value ? flat.value.findIndex((o) => o.value === matched.value!.value) : 0
  activeIndex.value = Math.min(Math.max(current + delta, 0), Math.max(props.options.length - 1, 0))
  open.value = true
}

const toggle = () => {
  if (open.value) {
    open.value = false
    return
  }
  openWithActive()
}

const choose = (value: string) => {
  emit('update:modelValue', value)
  open.value = false
}

const onTriggerKeydown = (e: KeyboardEvent) => {
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    openWithActive(e.key === 'ArrowUp' ? -1 : 0)
  } else if (e.key === 'Escape') {
    open.value = false
  }
}

const onPanelKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    e.preventDefault()
    open.value = false
    return
  }
  if (e.key === 'Tab') {
    open.value = false
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeIndex.value = Math.min(activeIndex.value + 1, flat.value.length - 1)
    return
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = Math.max(activeIndex.value - 1, 0)
    return
  }
  if (e.key === 'Enter') {
    e.preventDefault()
    const picked = flat.value[activeIndex.value]
    if (picked) choose(picked.value)
  }
}
</script>

<style scoped lang="scss">
.ee-select {
  position: relative;
  display: block;
  width: 100%;
}

.ee-select-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  padding: 9px 11px;
  color: var(--ee-text);
  background: var(--ee-bg);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font-size: 13.5px;
  text-align: left;
  transition:
    border-color var(--ee-transition),
    background var(--ee-transition);
}

.ee-select-trigger:hover {
  border-color: color-mix(in srgb, var(--ee-primary) 40%, var(--ee-border));
}

.ee-select-trigger[aria-expanded='true'] {
  background: var(--ee-surface);
  border-color: var(--ee-primary);
}

.ee-select-value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ee-select-value.is-placeholder {
  color: var(--ee-text-muted);
}

.ee-select-caret {
  flex: 0 0 auto;
  color: var(--ee-text-muted);
  font-size: 10px;
}

/* 面板：与 composer 的角色菜单同一套语言（surface + border + 软阴影）。
   坐标由 place() 以内联 style 给出（fixed），所以这里不写 top/left。 */
.ee-select-panel {
  z-index: 1200;
  overflow-y: auto;
  padding: 6px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: var(--ee-shadow-soft);
}

.ee-select-group-label {
  margin: 6px 8px 4px;
  color: var(--ee-text-muted);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.14em;
}

.ee-select-option {
  display: block;
  width: 100%;
  padding: 7px 8px;
  color: inherit;
  background: transparent;
  border: 0;
  border-radius: var(--ee-radius-sm);
  font-size: 13px;
  text-align: left;
  transition: background var(--ee-transition);
}

.ee-select-option.is-active {
  background: var(--ee-surface-muted);
}

.ee-select-option.is-selected {
  color: var(--ee-primary);
  font-weight: 600;
}
</style>
