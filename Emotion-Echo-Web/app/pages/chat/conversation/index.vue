<template>
  <div class="conversation-container">
    <!-- 折叠状态下的头部 -->
    <div class="header-fold" :class="$device.isMobile ? 'header-fold-mobile' : ''">
      <h3>情绪回音</h3>
      <el-button :icon="Memo" size="small" @click="foldAndUnfoldMenu" />
    </div>
    <!-- 侧边栏 -->
    <div class="list-container" ref="menuRef" :class="{ 'list-container-fold': isMenuFolded }">
      <div class="header">
        <h3>情绪回音</h3>
        <el-button :icon="Memo" @click="foldAndUnfoldMenu" />
      </div>
      <el-button
        type="primary"
        class="new-conversation-btn"
        :icon="ChatSquare"
        @click="startNewConversation"
        >新对话</el-button
      >
      <el-divider border-style="dashed" class="divider" />
      <ClientOnly>
        <el-skeleton :rows="8" animated v-if="isLoading && conversationItems.length === 0" />
        <el-empty
          description="暂无历史数据"
          v-else-if="!isLoading && conversationItems.length === 0"
        />
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
              <el-dropdown
                trigger="click"
                @command="(cmd: string) => handleMenuCommand(cmd, item)"
                @click.stop
              >
                <el-button :icon="MoreFilled" circle size="small" class="more-btn" @click.stop />
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item :command="item.isTop ? 'unpin' : 'pin'">
                      {{ item.isTop ? '取消置顶' : '置顶' }}
                    </el-dropdown-item>
                    <el-dropdown-item command="rename"> 重命名 </el-dropdown-item>
                    <el-dropdown-item command="delete" divided class="delete-item">
                      删除
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </div>
        </div>
        <template #fallback>
          <el-skeleton :rows="8" animated />
        </template>
      </ClientOnly>
    </div>
    <main class="main" :class="$device.isMobile ? 'main-mobile' : ''">
      <NuxtPage></NuxtPage>
    </main>
  </div>

  <!-- 重命名弹窗 -->
  <ClientOnly>
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名会话"
      width="300px"
      :close-on-click-modal="false"
    >
      <el-input
        v-model="renameValue"
        placeholder="请输入新名称"
        maxlength="50"
        show-word-limit
        @keyup.enter="submitRename"
      />
      <template #footer>
        <el-button @click="renameDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitRename">确定</el-button>
      </template>
    </el-dialog>
  </ClientOnly>
</template>

<script setup lang="ts">
import { ChatSquare, Memo, MoreFilled } from '@element-plus/icons-vue'

const route = useRoute()
const conversationStore = useConversationStore()

// ==================== 折叠控制 ====================
const isMenuFolded = ref(false)
const foldAndUnfoldMenu = () => {
  isMenuFolded.value = !isMenuFolded.value
}

// ==================== 新对话 ====================
const isNewConversation = computed(() => {
  return route.params.id === undefined
})
const startNewConversation = () => {
  if (isNewConversation.value) {
    return
  }
  navigateTo({ name: 'chat-conversation-new' })
}

// ==================== 数据加载 ====================
const isLoading = ref(false)
onMounted(async () => {
  isLoading.value = true
  await conversationStore.fetchConversations()
  isLoading.value = false
})

// ==================== 时间分组 ====================
const getTimeGroup = (updatedAt: string, isTop: boolean): string => {
  if (isTop) return '置顶'
  
  const now = new Date()
  const itemDate = new Date(updatedAt)
  
  // 使用绝对日期比较：同一年月日才算"今天"
  const nowYear = now.getFullYear()
  const nowMonth = now.getMonth()
  const nowDate = now.getDate()
  
  const itemYear = itemDate.getFullYear()
  const itemMonth = itemDate.getMonth()
  const itemDay = itemDate.getDate()
  
  if (itemYear === nowYear && itemMonth === nowMonth && itemDay === nowDate) {
    return '今天'
  }
  
  // 计算相差天数
  const diffMs = now.getTime() - itemDate.getTime()
  const diffDays = Math.floor(diffMs / (24 * 60 * 60 * 1000))
  
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

const conversationItems = computed<ConversationItem[]>(() => {
  return conversationStore.conversationList.map((c) => ({
    key: c.id,
    label: c.title,
    timestamp: new Date(c.updatedAt).getTime(),
    group: getTimeGroup(c.updatedAt, c.isTop),
    isTop: c.isTop
  }))
})

// ==================== 分组排序 ====================
const groupOrder = ['置顶', '今天', '昨日', '一周内', '三十天内', '更早']

const groupedConversations = computed(() => {
  const groups = new Map<string, ConversationItem[]>()

  conversationItems.value.forEach((item) => {
    if (!groups.has(item.group)) {
      groups.set(item.group, [])
    }
    groups.get(item.group)!.push(item)
  })

  // 按 groupOrder 排序
  return groupOrder
    .filter((name) => groups.has(name))
    .map((name) => ({
      name,
      items: groups.get(name)!
    }))
})

// ==================== 选中变更 ====================
const handleActiveChange = (key: string) => {
  navigateTo({ name: 'chat-conversation-detail', params: { id: key } })
}

// ==================== 菜单操作 ====================
const handleMenuCommand = (command: string, item: ConversationItem) => {
  switch (command) {
    case 'pin':
    case 'unpin':
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
  ElMessageBox.confirm('删除后将不可恢复!', '删除会话', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(() => {
    conversationStore.deleteConversation(id)
  })
}

// ==================== 重命名弹窗 ====================
const renameDialogVisible = ref(false)
const renameConversationId = ref('')
const renameValue = ref('')

const openRenameDialog = (id: string, currentTitle: string) => {
  renameConversationId.value = id
  renameValue.value = currentTitle
  renameDialogVisible.value = true
}

const submitRename = async () => {
  if (!renameValue.value.trim()) {
    ElNotification({
      type: 'error',
      title: '修改失败',
      message: '标题不能为空'
    })
    return
  }
  await conversationStore.updateConversationTitle(
    renameConversationId.value,
    renameValue.value.trim()
  )
  renameDialogVisible.value = false
}
</script>

<style scoped lang="scss">
@use 'sass:color';

.conversation-container {
  width: 100%;
  height: 100%;
  background-color: #ffffff;
  border-radius: $radius-lg;
  box-shadow: $box-shadow;
  display: flex;

  .main {
    flex: 1;
    padding: 10px;
  }

  .main-mobile {
    position: fixed;
    width: calc(100% - 40px);
    left: 20px;
  }
}

.header-fold {
  position: fixed;
  display: flex;
  padding: 10px;
  border-radius: $radius-lg;
  justify-content: space-between;
  align-items: center;
  width: 140px;
  background-color: color.adjust($bg-color, $alpha: -0.3);
  top: 30px;
  left: 170px;
  z-index: 1;
  color: $font-color-light;
  font-size: 12px;
}

.header-fold-mobile {
  top: 89px;
  left: 30px;
}

.list-container {
  z-index: 2;
  transition: all 0.5s ease-in-out;
  background-color: $bg-color;
  border-radius: $radius-lg;
  padding: 20px;
  width: 18vw;
  box-shadow: $box-shadow;
  overflow: hidden;
  flex-shrink: 0;
  min-width: 220px;

  .header {
    align-items: center;
    display: flex;
    margin-bottom: 20px;
    justify-content: space-between;

    h3 {
      color: $font-color-light;
      white-space: nowrap;
    }
  }

  .new-conversation-btn {
    width: 100%;
    height: 40px;
  }
}

// 折叠状态样式
.list-container-fold {
  width: 0;
  padding: 20px 0;
  min-width: 0;
}

.divider {
  margin-bottom: 0;
}

// 自定义会话列表样式
.conversation-list {
  max-height: calc(100% - 140px);
  overflow-y: auto;

  &::-webkit-scrollbar {
    width: 4px;
  }

  &::-webkit-scrollbar-thumb {
    background-color: #d1d5db;
    border-radius: 2px;
  }
}

.conversation-group {
  margin-bottom: 8px;
}

.group-title {
  color: $font-color-light;
  font-size: 14px;
  margin-bottom: 10px;
  margin-top: 10px;
  padding: 0 4px;
}

.conversation-item {
  border-radius: $radius-lg;
  margin-bottom: 8px;
  padding: 10px 12px;
  transition: all 0.3s ease;
  background-color: #fff;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  box-sizing: border-box;

  &:hover {
    background-color: #fff;
    box-shadow: $box-shadow;
    transform: translateY(-2px);
  }

  &.active {
    background-color: #fff;
    box-shadow: $box-shadow;
  }

  .item-label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 14px;
    color: #333;
  }

  .more-btn {
    opacity: 0;
    transition: opacity 0.2s;
    margin-left: 8px;
  }

  &:hover .more-btn {
    opacity: 1;
  }
}

.delete-item {
  color: #f56c6c;
}
</style>
