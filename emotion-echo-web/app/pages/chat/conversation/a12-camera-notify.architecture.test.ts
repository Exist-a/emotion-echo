// a12-camera-notify.test.ts — /new & /[id] 页摄像头错误用 notify 而非 window.alert
//
// 背景：F-119 修了 useFaceEmotion 抛具体 Error 后，调用方必须用 notify 显示而非 window.alert
//   - window.alert 是阻塞弹窗，体验差，阻塞主线程
//   - notify 与全站 UI 风格一致（success/error/warning/info 四态）
//
// 修法：
//   - new.vue toggleCamera catch 块：notify('摄像头开启失败', err?.message, 'error')（已是 notify）
//   - [id].vue:297 toggleCamera catch 块：window.alert → notify
//
// 契约钉（2 项）：
//   1. [id].vue toggleCamera catch 块必须用 notify 而非 window.alert
//   2. [id].vue 整体 window.alert 使用数应大幅减少（已知有上传/AI 失败等场景）——
//      本测试只钉 toggleCamera 一处，其它场景属 UX 范畴待评估

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const src = readFileSync(
  resolve(__dirname, './[id].vue'),
  'utf8',
)

describe('[id].vue · 摄像头错误用 notify 而非 window.alert', () => {
  it('toggleCamera catch 块用 notify（不再是 window.alert）', () => {
    // 提取 toggleCamera 函数体：以 `const toggleCamera = async` 起始，到下一个顶层 const voiceRecorder 之前
    const startIdx = src.indexOf('const toggleCamera = async')
    expect(startIdx, '必须存在 toggleCamera 函数').toBeGreaterThanOrEqual(0)
    const voiceRecorderIdx = src.indexOf('const voiceRecorder =', startIdx)
    expect(voiceRecorderIdx, '必须存在 toggleCamera 之后的 voiceRecorder 顶层声明').toBeGreaterThan(startIdx)
    const fnBody = src.slice(startIdx, voiceRecorderIdx)
    // fnBody 内 catch 块必须含 notify 调用
    expect(fnBody, 'toggleCamera 内必须有 notify 调用').toMatch(/notify\(/)
    // 不应再有 window.alert 函数调用（注释里的字符串不算）
    expect(fnBody, 'toggleCamera 内不应再有 window.alert( 调用').not.toMatch(/window\.alert\(/)
  })
})
