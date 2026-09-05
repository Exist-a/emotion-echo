<template>
  <div class="conversation-page" :class="{ 'is-mobile': $device.isMobile, 'is-folded': isMenuFolded }">
    <aside class="sidebar" :class="{ 'is-folded': isMenuFolded }">
      <header class="sidebar-header">
        <button
          v-if="!$device.isMobile"
          class="icon-btn fold-btn"
          type="button"
          :aria-label="isMenuFolded ? '展开会话列表' : '折叠会话列表'"
          @click="foldAndUnfoldMenu"
        >
          <span class="fold-arrow" :class="{ 'is-folded': isMenuFolded }" aria-hidden="true">‹</span>
        </button>
        <div v-else class="icon-btn-spacer" />
        <h3 class="sidebar-title">最近的对话</h3>
        <button class="icon-btn new-btn" type="button" aria-label="开始新的对话" @click="startNewConversation">
          <span aria-hidden="true">+</span>
        </button>
      </header>

      <div class="sidebar-body">
        <div v-if="isLoading" class="loading-state">
          <div v-for="i in 6" :key="i" class="skeleton-row" :style="{ width: 60 + Math.random() * 30 + '%' }" />
        </div>
        <div v-else-if="!isLoading && conversationItems.length === 0" class="empty-state">
          <span class="empty-mark">○</span>
          <p>还没有对话，从一句话开始吧</p>
        </div>
        <div v-else class="conversation-list">
          <div v-for="group in groupedConversations" :key="group.name" class="conversation-group">
            <div class="group-title">{{ group.name }}</div>
            <div
              v-for="item in group.items"
              :key="item.key"
              class="conversation-item"
              :class="{ active: item.key === route.params.id }"
              @click="handleActiveChange(item.key)"
            >
              <span class="item-label">{{ item.label }}</span>
              <div class="more-wrap" @click.stop>
                <button class="icon-btn more-btn" type="button" :aria-label="`对「${item.label}」更多操作`" @click.stop="toggleMore(item.key)">
                  <span class="more-dots" aria-hidden="true">
                    <span></span><span></span><span></span>
                  </span>
                </button>
                <ul v-if="openMoreKey === item.key" class="more-menu" role="menu">
                  <li role="menuitem" @click="onMenuCommand('pin', item)">
                    <span class="more-menu-icon" aria-hidden="true">·</span>
                    {{ item.isTop ? '取消置顶' : '置顶' }}
                  </li>
                  <li role="menuitem" @click="onMenuCommand('rename', item)">重命名</li>
                  <li role="menuitem" class="danger" @click="onMenuCommand('delete', item)">删除</li>
                </ul>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <button
      v-if="!$device.isMobile"
      class="sidebar-expand"
      type="button"
      aria-label="展开会话列表"
      @click="foldAndUnfoldMenu"
    >
      <span aria-hidden="true">›</span>
    </button>

    <main class="chat-area"><NuxtPage /></main>

    <Teleport v-if="renameDialogVisible" to="body">
      <div class="modal-backdrop" @click.self="renameDialogVisible = false">
        <div class="modal-card" role="dialog" aria-modal="true" aria-labelledby="rename-title">
          <h3 id="rename-title">重命名会话</h3>
          <input
            ref="renameInputRef"
            v-model="renameValue"
            class="modal-input"
            placeholder="给这段对话一个名字"
            maxlength="50"
            @keyup.enter="submitRename"
          />
          <div class="modal-meta"><span>{{ renameValue.length }}/50</span></div>
          <div class="modal-actions">
            <button class="btn btn-ghost" type="button" @click="renameDialogVisible = false">取消</button>
            <button class="btn btn-primary" type="button" :disabled="!renameValue.trim()" @click="submitRename">保存名称</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
const route = useRoute()
const router = useRouter()
const conversationStore = useConversationStore()

const isMenuFolded = ref(false)
const foldAndUnfoldMenu = () => {
  isMenuFolded.value = !isMenuFolded.value
  openMoreKey.value = null
}

const isNewConversation = computed(() => route.params.id === undefined)
const startNewConversation = () => {
  if (isNewConversation.value) return
  navigateTo({ name: 'chat-conversation-new' })
}

const isLoading = ref(false)
onMounted(async () => {
  isLoading.value = true
  await conversationStore.fetchConversations()
  isLoading.value = false
})

const getTimeGroup = (updatedAt: string, isTop: boolean): string => {
  if (isTop) return '置顶'
  const now = new Date()
  const itemDate = new Date(updatedAt)
  const nowYear = now.getFullYear()
  const nowMonth = now.getMonth()
  const nowDate = now.getDate()
  const itemYear = itemDate.getFullYear()
  const itemMonth = itemDate.getMonth()
  const itemDateValue = itemDate.getDate()
  if (itemYear === nowYear && itemMonth === nowMonth && itemDateValue === nowDate) return '今天'
  const diff = now.getTime() - itemDate.getTime()
  const diffDays = Math.floor(diff / (24 * 60 * 60 * 1000))
  if (diffDays === 1) return '昨日'
  if (diffDays <= 7) return '一周内'
  if (diffDays <= 30) return '三十天内'
  return '更早'
}

interface ConversationItem {
  key: string
  label: string
  timestamp: number
  group: string
  isTop: boolean
}

const conversationItems = computed<ConversationItem[]>(() =>
  conversationStore.conversationList.map((c) => ({
    key: c.id,
    label: c.title,
    timestamp: new Date(c.updatedAt).getTime(),
    group: getTimeGroup(c.updatedAt, c.isTop),
    isTop: c.isTop
  }))
)

const groupOrder = ['置顶', '今天', '昨日', '一周内', '三十天内', '更早']
const groupedConversations = computed(() => {
  const groups = new Map<string, ConversationItem[]>()
  conversationItems.value.forEach((item) => {
    if (!groups.has(item.group)) groups.set(item.group, [])
    groups.get(item.group)!.push(item)
  })
  return groupOrder
    .filter((name) => groups.has(name))
    .map((name) => ({ name, items: groups.get(name)! }))
})

const handleActiveChange = (key: string) => {
  navigateTo({ name: 'chat-conversation-detail', params: { id: key } })
}

// 自定义"更多"菜单（点击展开，再次点击或点击外部关闭）
const openMoreKey = ref<string | null>(null)
const toggleMore = (key: string) => {
  openMoreKey.value = openMoreKey.value === key ? null : key
}
const closeMore = () => (openMoreKey.value = null)
onMounted(() => document.addEventListener('click', closeMore))
onBeforeUnmount(() => document.removeEventListener('click', closeMore))

const onMenuCommand = (command: string, item: ConversationItem) => {
  openMoreKey.value = null
  switch (command) {
    case 'pin':
      conversationStore.togglePinConversation(item.key)
      break
    case 'rename':
      openRenameDialog(item.key, item.label)
      break
    case 'delete':
      handleDelete(item.key)
      break
  }
}

const handleDelete = (id: string) => {
  if (import.meta.client) {
    const ok = window.confirm('删除后将不可恢复')
    if (ok) conversationStore.deleteConversation(id)
  }
}

const renameDialogVisible = ref(false)
const renameValue = ref('')
const renameConversationId = ref('')
const renameInputRef = ref<HTMLInputElement | null>(null)

const openRenameDialog = (id: string, currentTitle: string) => {
  renameConversationId.value = id
  renameValue.value = currentTitle
  renameDialogVisible.value = true
  nextTick(() => renameInputRef.value?.focus())
}

const submitRename = async () => {
  if (!renameValue.value.trim()) {
    alert('标题不能为空')
    return
  }
  await conversationStore.updateConversationTitle(renameConversationId.value, renameValue.value.trim())
  renameDialogVisible.value = false
}

// 点击外部区域关闭更多菜单：handler 已经通过 document listener 监听；
// Esc 关闭
const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') openMoreKey.value = null }
onMounted(() => document.addEventListener('keydown', onKey))
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))
</script>

<style scoped lang="scss">
.conversation-page {
  position: relative;
  display: flex;
  flex: 1;
  min-height: 0;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  overflow: hidden;
}

.sidebar {
  display: flex;
  flex-direction: column;
  width: 250px;
  flex-shrink: 0;
  padding: 16px 12px 12px;
  background: var(--ee-surface);
  border-right: 1px solid var(--ee-border);
  transition: width var(--ee-transition), padding var(--ee-transition), border-color var(--ee-transition);
  overflow: hidden;
}

.sidebar.is-folded {
  width: 0;
  padding-left: 0;
  padding-right: 0;
  border-right-color: transparent;
}

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  flex-shrink: 0;
}

.icon-btn,
.icon-btn-spacer {
  width: 30px;
  height: 30px;
  flex-shrink: 0;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--ee-text-muted);
  background: transparent;
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  cursor: pointer;
  transition: color var(--ee-transition), border-color var(--ee-transition), background var(--ee-transition);
}

.icon-btn:hover {
  color: var(--ee-primary);
  border-color: var(--ee-primary);
  background: var(--ee-primary-soft);
}

.fold-arrow {
  font-size: 16px;
  line-height: 1;
  transition: transform var(--ee-transition);
}

.fold-arrow.is-folded {
  transform: rotate(180deg);
}

.sidebar-title {
  flex: 1;
  margin: 0;
  color: var(--ee-text);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: -0.01em;
  white-space: nowrap;
  text-overflow: ellipsis;
  overflow: hidden;
}

.new-btn {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  border-color: transparent;
  font-size: 18px;
  line-height: 1;
}

.new-btn:hover {
  background: color-mix(in srgb, var(--ee-primary-soft) 60%, var(--ee-primary));
  border-color: transparent;
}

.sidebar-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.loading-state { display: flex; flex-direction: column; gap: 10px; padding: 4px; }
.skeleton-row { height: 30px; background: var(--ee-surface-muted); border-radius: var(--ee-radius-sm, 6px); }

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 36px 8px;
  color: var(--ee-text-muted);
}
.empty-mark { font-size: 28px; opacity: 0.6; }
.empty-state p { margin: 0; font-size: 12px; }

.conversation-list { display: flex; flex-direction: column; }
.conversation-group { margin-bottom: 14px; }
.group-title {
  margin: 0 6px 6px;
  color: var(--ee-text-muted);
  font-size: 11px;
  letter-spacing: 0.04em;
}

.conversation-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 8px 6px 8px 10px;
  color: var(--ee-text-muted);
  border-radius: var(--ee-radius-md);
  cursor: pointer;
  transition: color var(--ee-transition), background var(--ee-transition);
}

.conversation-item:hover { color: var(--ee-text); background: var(--ee-surface-muted); }
.conversation-item.active {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  font-weight: 600;
}

.item-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.more-wrap { position: relative; }

.more-btn {
  opacity: 0;
  width: 26px;
  height: 26px;
  border-color: transparent;
  background: transparent;
  font-size: 14px;
  color: currentColor;
}

.conversation-item:hover .more-btn,
.conversation-item:focus-within .more-btn { opacity: 1; }

.more-dots {
  display: inline-flex;
  flex-direction: column;
  gap: 2px;
  align-items: center;
}

.more-dots span {
  display: block;
  width: 3px;
  height: 3px;
  background: currentColor;
  border-radius: 50%;
}

.more-menu {
  position: absolute;
  top: 100%;
  right: 0;
  z-index: 30;
  min-width: 130px;
  margin: 4px 0 0;
  padding: 4px;
  list-style: none;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  box-shadow: 0 8px 24px rgba(32, 37, 34, 0.12);
}

.more-menu li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  color: var(--ee-text);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  transition: background var(--ee-transition);
}

.more-menu li:hover { background: var(--ee-surface-muted); }
.more-menu li.danger { color: var(--ee-accent); }
.more-menu-icon { color: var(--ee-text-muted); }

.sidebar-expand {
  position: absolute;
  top: 16px;
  left: 16px;
  z-index: 5;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: var(--ee-text-muted);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  transition: color var(--ee-transition), border-color var(--ee-transition);
}

.sidebar-expand:hover {
  color: var(--ee-primary);
  border-color: var(--ee-primary);
}

.chat-area {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(20, 27, 23, 0.45);
  backdrop-filter: blur(2px);
}

.modal-card {
  width: min(420px, 100%);
  padding: 20px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: 0 12px 36px rgba(32, 37, 34, 0.15);
}

.modal-card h3 { margin: 0 0 12px; font-size: 16px; font-weight: 600; }

.modal-input {
  width: 100%;
  padding: 10px 12px;
  color: var(--ee-text);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font-size: 14px;
  outline: none;
  transition: border-color var(--ee-transition), box-shadow var(--ee-transition);
}

.modal-input:focus {
  border-color: var(--ee-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ee-primary) 25%, transparent);
}

.modal-meta { display: flex; justify-content: flex-end; margin-top: 4px; color: var(--ee-text-muted); font-size: 11px; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 38px;
  padding: 0 18px;
  font-size: 13px;
  font-weight: 600;
  border-radius: var(--ee-radius-md);
  border: 1px solid transparent;
  cursor: pointer;
  transition: background var(--ee-transition), color var(--ee-transition), border-color var(--ee-transition);
}

.btn-primary { background: var(--ee-primary); color: #fff; border-color: var(--ee-primary); }
.btn-primary:hover { background: var(--ee-primary-hover); border-color: var(--ee-primary-hover); }
.btn-primary:disabled { background: var(--ee-border); border-color: var(--ee-border); cursor: not-allowed; }

.btn-ghost { background: transparent; color: var(--ee-text-muted); border-color: var(--ee-border); }
.btn-ghost:hover { color: var(--ee-text); border-color: var(--ee-border); background: var(--ee-surface-muted); }

@media (max-width: 768px) {
  .conversation-page { min-height: calc(100vh - 118px); border: 0; border-radius: 0; }
  .sidebar {
    position: fixed;
    top: 78px;
    left: 12px;
    bottom: 12px;
    z-index: 18;
    width: min(78vw, 280px);
    border: 1px solid var(--ee-border);
    border-radius: var(--ee-radius-lg);
    box-shadow: var(--ee-shadow-soft);
  }
  .sidebar.is-folded { display: none; }
  .sidebar-expand { top: 88px; left: 12px; }
}
</style>
