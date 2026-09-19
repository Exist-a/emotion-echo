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

  // === 4. 头像上传入口 ===
  // 演进：先是 el-upload(action="" + on-success) → 改 :http-request → 最终发现
  // Element Plus 根本不是项目依赖，el-upload 整体未解析 → 换原生 input[type=file]。
  it('头像上传必须用原生 input[type="file"]', () => {
    expect(
      /<input[^>]*type="file"/.test(pageSrc),
      'E2E-11: 头像上传入口必须是原生 <input type="file">。' +
        'el-upload 未解析（项目无 element-plus 依赖），其 :http-request 等 prop 也不会生效。',
    ).toBe(true)
  })

  it('文件 input 必须限定图片类型', () => {
    const fileInput = pageSrc.match(/<input[^>]*type="file"[^>]*>/) || []
    expect(
      fileInput.some((m) => /accept="image\/\*"/.test(m)),
      'E2E-11: 文件 input 应带 accept="image/*"，避免用户误选非图片文件。',
    ).toBe(true)
  })

  it('不得残留旧的 handleAvatarSuccess 处理器', () => {
    expect(
      /handleAvatarSuccess/.test(pageSrc),
      'E2E-11: handleAvatarSuccess 依赖 el-upload 的 on-success（已改为原生 input），应删除。',
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

  // === 8. 不得使用 Element Plus 组件（项目未安装 element-plus）===
  // 浏览器实测（2026-09-19）：控制台报
  //   [Vue warn]: Failed to resolve component: el-dialog
  //   [Vue warn]: Failed to resolve component: el-upload
  // 后果：<el-dialog> 被当作普通自定义元素内联渲染 ⇒ "弹框" 恒可见；
  // 且 <template #footer> 命名插槽对非组件被静默丢弃 ⇒
  // 保存资料 / 确认退出 / 取消 / 留下 按钮**一个都不存在**（实测 count 全为 0），
  // 用户无法保存资料、无法从本页退出登录。
  //
  // 注意：只检查 <template> 段（script 注释里会提到这些组件名，不应误报）。
  const templateSrc = pageSrc.split('<script setup')[0] || ''

  it('不得引用 el-dialog（Element Plus 非项目依赖）', () => {
    expect(
      /<el-dialog[\s>]/.test(templateSrc),
      'E2E-11: 项目只依赖 @element-plus/icons-vue，没有 element-plus 本体，' +
        '<el-dialog> 无法解析。应改用项目原生弹框模式（见 SecurityQuestionDialog.vue：' +
        'Teleport + v-if overlay + role="dialog"）。',
    ).toBe(false)
  })

  it('不得引用 el-upload（Element Plus 非项目依赖）', () => {
    expect(
      /<el-upload[\s>]/.test(templateSrc),
      'E2E-11: <el-upload> 同样无法解析 → 头像上传入口渲染为普通自定义元素。' +
        '应改用原生 <input type="file">。',
    ).toBe(false)
  })

  // === 9. 弹框 action 按钮必须真实存在 ===
  it('资料弹框必须有真实的"保存资料"按钮（非被丢弃的插槽内容）', () => {
    expect(
      /<button[^>]*>\s*保存资料\s*<\/button>/.test(pageSrc),
      'E2E-11: 保存资料必须是真的 <button> 元素。原实现放在 <template #footer> 里，' +
        '而宿主 <el-dialog> 未解析 ⇒ 命名插槽被静默丢弃，按钮在 DOM 中根本不存在（实测 count=0）。',
    ).toBe(true)
  })

  it('退出弹框必须有真实的"确认退出"按钮', () => {
    expect(
      /<button[^>]*>\s*确认退出\s*<\/button>/.test(pageSrc),
      'E2E-11: 确认退出必须是真的 <button> 元素（同上，原插槽内容被丢弃）。',
    ).toBe(true)
  })

  it('不得残留 <template #footer>（未解析组件下插槽必被丢弃）', () => {
    expect(
      /<template\s+#footer/.test(pageSrc),
      'E2E-11: <template #footer> 只在真实组件下才渲染；本页宿主组件未解析时静默丢弃，' +
        '导致所有操作按钮消失。原生弹框应把按钮直接写在内容里。',
    ).toBe(false)
  })

  it('原生弹框必须使用 role="dialog" + aria-modal（可访问性）', () => {
    const dialogRoles = pageSrc.match(/role="dialog"/g) || []
    expect(
      dialogRoles.length,
      'E2E-11: 两个原生弹框都应带 role="dialog"（对齐 SecurityQuestionDialog.vue 的项目约定）',
    ).toBeGreaterThanOrEqual(2)
    expect(/aria-modal="true"/.test(pageSrc), '原生弹框应带 aria-modal="true"').toBe(true)
  })

  // === 10. 页面必须自取用户资料 ===
  // 契约来源：app/middleware/auth.global.ts:51 明确写
  //   "userInfo 是页面元数据，由各页面 onMounted 自取（fetchUserInfo）"
  // 且 init.ts 的 fetchUserInfo 被 `if (userStore.isAuthenticated)` 门控，而
  // isAuthenticated 又要求 userInfo?.id 非空 —— 冷启动时 userInfo 为 null
  // ⇒ 该分支不进 ⇒ 资料永远不刷新。
  // 后果（浏览器实测 2026-09-19）：页面恒显兜底值"用户"/"18 岁"、ID 为空，
  // 编辑弹框回填的也是兜底值而非服务端真实资料。
  it('onMounted 必须自取用户资料（fetchUserInfo）', () => {
    const mountedMatch = pageSrc.match(/onMounted\s*\(\s*async\s*\(\)\s*=>\s*\{[\s\S]*?\n\}\)/)
    const body = mountedMatch?.[0] || ''
    expect(body, 'onMounted 应能找到').not.toBe('')
    expect(
      /fetchUserInfo\s*\(/.test(body),
      'E2E-11: 本页 onMounted 必须 await userStore.fetchUserInfo()。' +
        'auth.global.ts 约定各页面自取 userInfo；本页只 fetchBehaviorData() 不取资料，' +
        '导致昵称/年龄/ID 显示兜底值，编辑弹框回填错误数据。',
    ).toBe(true)
  })

  // === 11. 表单字段必须有可见标签 ===
  // .ee-field / .ee-input 全仓无 CSS 定义（global.scss 只定义了 .ee-btn 家族），
  // 因此 data-label 属性不会渲染成标签 —— 弹框里只有裸输入框，用户不知道填什么。
  it('资料表单字段必须有可见标签（不能只靠 data-label 属性）', () => {
    expect(
      /\.ee-field\b[^{]*\{[\s\S]{0,200}(attr\(data-label\)|::before)/.test(pageSrc) ||
        /\.profile-form\b[\s\S]{0,200}label/.test(pageSrc) ||
        /<span[^>]*class="[^"]*field-label/.test(pageSrc),
      'E2E-11: `.ee-field` 全仓无样式定义，`data-label` 属性不会渲染。' +
        '表单字段必须有真实可见的标签元素或本地 CSS（::before + attr(data-label)）。',
    ).toBe(true)
  })

  // === 12. store getter 必须包成 computed（否则快照化，永不更新）===
  // 浏览器实测（2026-09-19）：Pinia 里 userInfo={"nickname":"Echo User"} 已就位，
  // 但 DOM 恒显 "用户"。根因：`defineStore(setup)` 的返回值经 store 代理后 ref 被
  // 解包 ⇒ `const nickname = userStore.getNickname` 拿到的是**当时的普通字符串**
  // （setup 时 userInfo 还是 null → 冻结在兜底值 "用户"），既不是 ref 也不参与渲染追踪。
  // 连带后果：editInfo() 里的 `nickname.value` 是 undefined ⇒ 弹框输入框恒为空。
  it('store getter 必须包成 computed（不得裸赋 const，否则快照化）', () => {
    // 只丢弃「整行注释」，不做行内剥离 —— 否则 accept="image/*" 里的 `/*`
    // 会被当成块注释起点，把后续代码整段吞掉（本轮已踩过该坑）。
    const code = pageSrc
      .split('\n')
      .filter((l) => {
        const t = l.trim()
        return !(t.startsWith('//') || t.startsWith('*') || t.startsWith('/*'))
      })
      .join('\n')
    for (const getter of ['getNickname', 'getAvatarPath', 'getAge', 'getId']) {
      expect(
        new RegExp(`const\\s+\\w+\\s*=\\s*userStore\\.${getter}\\b`).test(code),
        `E2E-11: \`const x = userStore.${getter}\` 会快照化（Pinia 解包 ref）⇒ 值永不更新，` +
          '且 x.value 为 undefined。必须写成 `computed(() => userStore.' +
          getter +
          ')`。',
      ).toBe(false)
      expect(
        new RegExp(`computed\\(\\s*\\(\\)\\s*=>\\s*userStore\\.${getter}`).test(code),
        `E2E-11: ${getter} 必须包成 computed(() => userStore.${getter}) 才能随 store 更新。`,
      ).toBe(true)
    }
  })
})
