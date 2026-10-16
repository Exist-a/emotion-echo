#!/usr/bin/env python3
"""
test_check_git_layout.py — 验证 check_git_layout.py 的 `.git` 判定在 **git worktree** 下不假红

背景（CL-F-06，2026-10-14 A 轨实测）：
  `git worktree` 检出的工作区里，`.git` 是**文件**（内容为 `gitdir: <主仓路径>`），不是目录。
  旧实现 `git_dir.is_dir()` 在 worktree 下恒为 False ⇒ `check_git_layout.py --strict` 恒红，
  而同一命令在主工作区 exit 0 —— 守卫在 worktree 方案下失去意义。

本测试按 TDD 钉住判定逻辑本身（工具脚本也是代码）：
  1. `.git` 是目录            → 已初始化（普通 clone）
  2. `.git` 是 gitdir 指针文件 → 已初始化（worktree）
  3. `.git` 是普通文件/垃圾内容 → 未初始化（别把随便一个文件当仓库）
  4. 没有 `.git`               → 未初始化

无外部依赖（stdlib + 同目录 check_git_layout.py）。

跑：python scripts/test_check_git_layout.py
"""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).parent.resolve()
TOOL = HERE / "check_git_layout.py"


def load_tool():
    spec = importlib.util.spec_from_file_location("check_git_layout", TOOL)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(mod)
    return mod


class TestGitInitializedDetection(unittest.TestCase):
    """`is_git_initialized(root)` 必须同时认「目录」与「worktree 的 gitdir 指针文件」"""

    @classmethod
    def setUpClass(cls) -> None:
        cls.mod = load_tool()

    def test_function_exists(self) -> None:
        """判据必须抽成可单测的函数（否则只能靠起真 worktree 才测得到）"""
        self.assertTrue(
            hasattr(self.mod, "is_git_initialized"),
            "check_git_layout.py 应导出 is_git_initialized(root) 供测试与复用",
        )

    def test_git_directory_is_initialized(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            (Path(td) / ".git").mkdir()
            self.assertTrue(self.mod.is_git_initialized(Path(td)))

    def test_gitdir_pointer_file_is_initialized(self) -> None:
        """worktree：`.git` 是内容以 `gitdir:` 开头的文件"""
        with tempfile.TemporaryDirectory() as td:
            fake_main = Path(td) / "main-repo" / ".git" / "worktrees" / "wt"
            (Path(td) / ".git").write_text(f"gitdir: {fake_main}\n", encoding="utf-8")
            self.assertTrue(self.mod.is_git_initialized(Path(td)))

    def test_plain_file_is_not_initialized(self) -> None:
        """随便一个文件不算仓库（不能只看 exists）"""
        with tempfile.TemporaryDirectory() as td:
            (Path(td) / ".git").write_text("not a git marker\n", encoding="utf-8")
            self.assertFalse(self.mod.is_git_initialized(Path(td)))

    def test_missing_git_is_not_initialized(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            self.assertFalse(self.mod.is_git_initialized(Path(td)))


class TestRealRepoStillPasses(unittest.TestCase):
    """回归：真实仓库（当前会话所在的工作区）本来就该判为已初始化"""

    def test_this_repo_is_initialized(self) -> None:
        mod = load_tool()
        repo_root = TOOL.parent.parent
        self.assertTrue(
            mod.is_git_initialized(repo_root),
            f"当前工作区被误判为未初始化：{repo_root}",
        )


if __name__ == "__main__":
    sys.exit(0 if unittest.main(exit=False).result.wasSuccessful() else 1)
