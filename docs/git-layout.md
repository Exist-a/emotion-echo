# Emotion-Echo 仓库布局规范

**日期**：2026-07-16
**适用**：本仓库所有协作者（人类 / AI Agent）

---

## 一、仓库拓扑

本仓库是 **monorepo**，采用 **主仓 + 4 个 git submodule** 的拓扑：

```
Emotion-Echo/  ←── 主仓（github.com/Exist-a/emotion-echo）
│
├── emotion-echo-shared/          # 主仓内容 ✅
├── emotion-echo-ai-svc/          # 主仓内容 ✅
├── emotion-echo-chat-svc/        # 主仓内容 ✅
├── emotion-echo-analytics-svc/   # 主仓内容 ✅
├── emotion-echo-assessment-svc/  # 主仓内容 ✅
├── emotion-echo-user-svc/        # 主仓内容 ✅
├── emotion-llm-service/          # 主仓内容 ✅
├── proto/                        # 主仓内容 ✅
├── deploy/                       # 主仓内容 ✅
├── docs/                         # 主仓内容 ✅
├── scripts/                      # 主仓内容 ✅
│
├── Emotion-Echo-Web/             # SUBMODULE → github.com/Exist-a/Emotion-Echo-Web
├── legacy/emotion-echo-gin/      # SUBMODULE → github.com/Exist-a/Emotion-Echo-Gin
├── Emotion-Echo-LLM/sensevoice-small/  # SUBMODULE → 本地占位（待 P0-A 时启用）
└── Emotion-Echo-LLM/XTTS/TTS/          # SUBMODULE → 本地占位（待 P0-A 时启用）
```

---

## 二、主仓 vs Submodule 的判定标准

| 判定问题 | 主仓 | Submodule |
|---------|------|-----------|
| 是否与后端/AI 编排紧密耦合？ | ✅ | |
| 是否会跨 5+ 个服务共享代码？ | ✅ | |
| 是否有自己的发布节奏、独立版本？ | | ✅ |
| 是否已有独立的 GitHub 仓库？ | | ✅ |
| 是否是 AI 模型服务（独立 GPU 环境）？ | | ✅ |

---

## 三、Submodule 列表

| Submodule 路径 | 来源 | 当前状态 |
|---------------|------|---------|
| `Emotion-Echo-Web/` | github.com/Exist-a/Emotion-Echo-Web | 已注册为 submodule（指向 GitHub 远程） |
| `legacy/emotion-echo-gin/` | github.com/Exist-a/Emotion-Echo-Gin | 已注册为 submodule（指向 GitHub 远程） |
| `Emotion-Echo-LLM/sensevoice-small/` | 本地占位 | 已注册为 submodule，远程 URL 待补 |
| `Emotion-Echo-LLM/XTTS/TTS/` | 本地占位 | 已注册为 submodule，远程 URL 待补 |

---

## 四、协作约定

### 4.1 克隆主仓
```bash
git clone https://github.com/Exist-a/emotion-echo.git
cd emotion-echo
git submodule update --init --recursive   # 拉取所有 submodule
```

### 4.2 在 submodule 内工作
```bash
cd Emotion-Echo-Web
# 在子仓内正常 add / commit / push
git add . && git commit -m "..." && git push origin main

# 回到主仓，**提升 submodule 引用**
cd ..
git add Emotion-Echo-Web
git commit -m "chore(submodule): bump Emotion-Echo-Web to <commit-sha>"
```

### 4.3 修改远程 URL（如果 submodule 用了占位）
```bash
git submodule set-url Emotion-Echo-LLM/sensevoice-small <real-url>
git submodule sync
```

### 4.4 添加新的 submodule
```bash
git submodule add <url> <path>
git add .gitmodules <path>
git commit -m "chore(submodule): add <name>"
```

---

## 五、本文件的演进

任何对仓库布局的修改（添加 / 移除 / 重组 submodule 或主仓内容）都必须：

1. 先在 `docs/git-layout.md` 提交 PR 修改
2. 通过 review 后再执行实际操作
3. 在 commit message 中引用本文档章节