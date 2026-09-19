import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// E2E-11 · 我的空间 architecture regression (static-source)
//
// 代码现状调查（2026-09-19）发现的 6 个前端缺陷：
// 1. `notify(...)` 被调用 11 次但从未 import（只解构了 notifySuccess/notifyError
//    且从未使用）→ 昵称校验失败时抛 ReferenceError，用户看不到任何提示
// 2. `<Plus />` 组件从未定义/import → Vue "Failed to resolve component"，头像上传按钮空白
// 3. `.ee-empty` / `.ee-skeleton` 无 v-if 条件 → 有数据时"暂无数据"占位符仍渲染
// 4. el-upload `action=""` + `:on-success` → 组件自带 XHR POST 到当前页地址，
//    自定义上传逻辑（handleUploadAvatar）依赖 on-success 触发但上传本身会失败
// 5. editInfo() 只置 dialogFormVisible，不回填表单 → 上次未保存的编辑残留
// 6. form 初始值赋的是 computed ref 对象（靠 ref() 深解包侥幸工作）
//
// 本测试钉死上述修复，防止回归。

const pageSrc = readFileSync('./app/pages/chat/user/index.vue', 'utf8')

describe('E2E-11 · 我的空间前端契约', () => {
  // === 1. notify 必须被 import ===
  it('notify 必须从 useNotify 显式 import（否则校验失败时 ReferenceError）', () => {
    expect(
      /import\s*\{[^}]*\bnotify\b[^}]*\}\s*from\s*['"]~\/composables\/useNotify['"]/.test(pageSrc),
      'E2E-11: 页面调用 notify(...) 11 次，必须 import { notify } from "~/composables/useNotify"；' +
        '原代码只 import { useNotify } 并解构出从未使用的 notifySuccess/notifyError，' +
        '裸 notify(...) 是未定义引用 → 昵称/年龄校验失败时抛 ReferenceError。',
    ).toBe(true)
  })

  it('不得残留未使用的 notifySuccess/notifyError 解构', () => {
    expect(
      /const\s*\{\s*success:\s*notifySuccess/.test(pageSrc),
      'E2E-11: 原 `const { success: notifySuccess, error: notifyError } = useNotify()` 中' +
        '两个变量从未被使用（全页用的是裸 notify），属死代码，应删除。',
    ).toBe(false)
  })

  // === 2. Plus 图标必须内联 SVG ===
  it('不得引用未定义的 <Plus /> 组件', () => {
    expect(
      /<Plus\s*\/>/.test(pageSrc),
      'E2E-11: <Plus /> 组件在本项目从未定义或 import（app/components 下无 icons 目录），' +
        'Vue 会报 "Failed to resolve component: Plus" → 上传按钮渲染为空。' +
        '项目约定用内联 SVG（见 chat/conversation/new.vue）。',
    ).toBe(false)
  })

  it('头像上传入口必须用内联 SVG 加号图标', () => {
    expect(
      pageSrc.includes('M12 5v14M5 12h14'),
      'E2E-11: 上传按钮应使用内联 SVG 加号（path "M12 5v14M5 12h14"），与项目 SVG 图标约定一致。',
    ).toBe(true)
  })

  // === 3. 空态 / 骨架屏必须条件渲染 ===
  it('ee-empty 必须有 v-if/v-else 条件（chartData.length === 0）', () => {
    expect(
      /v-else-if="chartData\.length\s*===\s*0"[\s\S]{0,80}ee-empty/.test(pageSrc),
      'E2E-11: `.ee-empty`（暂无数据）原本无 v-if，即使 3 个图表都渲染成功也同时显示' +
        '"暂无数据"占位符 → 用户看到图表 + 空态的诡异组合。必须用 v-else-if="chartData.length === 0" 条件渲染。',
    ).toBe(true)
  })

  it('ee-skeleton 必须有 v-if="isLoadingBehavior" 条件', () => {
    expect(
      /v-if="isLoadingBehavior"[\s\S]{0,80}ee-skeleton/.test(pageSrc),
      'E2E-11: `.ee-skeleton` 原本无 v-if（恒渲染）→ 加载完成后骨架屏仍在，页面永远像"加载中"。',
    ).toBe(true)
  })

  // === 4. el-upload 必须走自定义 http-request ===
  it('el-upload 必须使用 :http-request（不能依赖 action="" + on-success）', () => {
    expect(
      /:http-request="handleAvatarUpload"/.test(pageSrc),
      'E2E-11: el-upload 原配置 `action=""` + `:on-success="handleAvatarSuccess"`，' +
        '组件会自行 POST 到空 action（当前页地址）→ 上传失败则 on-success 永不触发，' +
        '自定义上传逻辑（handleUploadAvatar）成为死代码。' +
        '必须用 :http-request 完全接管上传。',
    ).toBe(true)
  })

  it('el-upload 不得残留 action="" 属性', () => {
    // 只检查 <template> 段（script 里的注释会提到 action=""，不应误报）
    const template = pageSrc.split('<script setup')[0] || ''
    expect(
      /\saction=/.test(template),
      'E2E-11: `action=""` 会让 el-upload 自带 XHR POST 到当前页 URL，必须移除。',
    ).toBe(false)
  })

  it('不得残留旧的 handleAvatarSuccess 处理器', () => {
    expect(
      /handleAvatarSuccess/.test(pageSrc),
      'E2E-11: handleAvatarSuccess 依赖 el-upload 的 on-success（已改走 http-request），应删除。',
    ).toBe(false)
  })

  // === 5. editInfo 必须回填表单 ===
  it('editInfo() 必须回填 nickname/avatarPath/age（防上次未保存编辑残留）', () => {
    const fnMatch = pageSrc.match(/const editInfo\s*=\s*\(\)\s*=>\s*\{[\s\S]*?\n\}/)
    const body = fnMatch?.[0] || ''
    expect(body, 'editInfo 函数应能找到').not.toBe('')
    for (const field of ['nickname', 'avatarPath', 'age']) {
      expect(
        new RegExp(`form\\.value\\.${field}\\s*=`).test(body),
        `E2E-11: editInfo() 必须在打开对话框时回填 form.value.${field}，` +
          '否则用户改了昵称后点"取消"，再次打开对话框仍显示上次的未保存值。',
      ).toBe(true)
    }
  })

  // === 6. form 初始值不得是 computed ref 对象 ===
  it('form 初始值不得直接赋 computed ref（as unknown as string 掩盖类型错误）', () => {
    expect(
      /nickname:\s*nickname\s+as\s+unknown\s+as\s+string/.test(pageSrc),
      'E2E-11: 原 `nickname: nickname as unknown as string` 把 computed ref 对象塞进 form，' +
        '靠 ref() 的深解包侥幸工作（脆弱）；应初始化为普通字面量，由 editInfo() 回填。',
    ).toBe(false)
  })

  // === 7. 错误提示不得为空 ===
  it('错误 notify 不得传空 message（用户看不到失败原因）', () => {
    const emptyNotifications = pageSrc.match(/notify\(\s*''\s*,\s*''\s*,\s*'error'/g) || []
    expect(
      emptyNotifications.length,
      `E2E-11: 发现 ${emptyNotifications.length} 处 notify('', '', 'error', ...) 传空消息，` +
        '用户只看到空 toast，无法判断失败原因。应传 error.message 或兜底文案。',
    ).toBe(0)
  })
})
