#!/usr/bin/env python3
"""
test_check_view_consistency.py — 验证 check_view_consistency.py 在 drift 时 FAIL

Round 1.4 TDD: 工具脚本本身也是代码，必须有测试。本测试：
1. 起一个临时目录，模拟 2 个文件定义同名 view 但内容不同
2. 跑 check_view_consistency.py → 期望 exit 1 + stderr 含 [FAIL]
3. 修正其中 1 个文件使内容一致 → 期望 exit 0

无外部依赖（仅用 stdlib + 同目录的 check_view_consistency.py）。

跑：python scripts/test_check_view_consistency.py
"""
from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).parent.resolve()
TOOL = HERE / "check_view_consistency.py"


def run_tool(repo_root: Path) -> tuple[int, str]:
    result = subprocess.run(
        [sys.executable, str(TOOL), "--repo-root", str(repo_root)],
        capture_output=True,
        text=True,
        timeout=30,
    )
    return result.returncode, (result.stdout + result.stderr)


class TestViewConsistency(unittest.TestCase):
    def test_drift_is_detected(self):
        """两个文件定义同名 view 但内容不同 → 工具 FAIL。"""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "deploy" / "db").mkdir(parents=True)
            (root / "svc_a" / "migrations").mkdir(parents=True)
            (root / "svc_b" / "migrations").mkdir(parents=True)
            # 1 个老的 deploy/db 定义
            (root / "deploy" / "db" / "04.sql").write_text(
                "CREATE OR REPLACE VIEW public.foo_v AS SELECT id, name FROM public.t;\n",
                encoding="utf-8",
            )
            # svc_a 与 svc_b 定义同名 view 但字段不同
            (root / "svc_a" / "migrations" / "001.sql").write_text(
                "CREATE OR REPLACE VIEW public.foo_v AS SELECT id, name FROM public.t;\n",
                encoding="utf-8",
            )
            (root / "svc_b" / "migrations" / "001.sql").write_text(
                "CREATE OR REPLACE VIEW public.foo_v AS SELECT id, name, age FROM public.t;\n",
                encoding="utf-8",
            )

            code, out = run_tool(root)
            self.assertEqual(code, 1, f"期望 exit 1, 实测 {code}\n{out}")
            self.assertIn("[FAIL] public.foo_v", out)

    def test_consistent_passes(self):
        """两个文件定义同名 view 且内容相同 → 工具 PASS。"""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "deploy" / "db").mkdir(parents=True)
            (root / "svc_a" / "migrations").mkdir(parents=True)
            (root / "svc_b" / "migrations").mkdir(parents=True)
            body = "CREATE OR REPLACE VIEW public.foo_v AS SELECT id FROM public.t;\n"
            (root / "deploy" / "db" / "04.sql").write_text(body, encoding="utf-8")
            (root / "svc_a" / "migrations" / "001.sql").write_text(body, encoding="utf-8")
            (root / "svc_b" / "migrations" / "001.sql").write_text(body, encoding="utf-8")

            code, out = run_tool(root)
            self.assertEqual(code, 0, f"期望 exit 0, 实测 {code}\n{out}")
            self.assertIn("[PASS]", out)

    def test_legacy_excluded(self):
        """legacy/ 目录不参与（Round 1.3 glob 改造的副作用）。"""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "svc_a" / "migrations").mkdir(parents=True)
            (root / "legacy" / "svc_old" / "migrations").mkdir(parents=True)
            (root / "svc_a" / "migrations" / "001.sql").write_text(
                "CREATE OR REPLACE VIEW public.bar_v AS SELECT id FROM public.t;\n",
                encoding="utf-8",
            )
            # legacy 定义"漂移"版本，期望被忽略
            (root / "legacy" / "svc_old" / "migrations" / "001.sql").write_text(
                "CREATE OR REPLACE VIEW public.bar_v AS SELECT id, extra FROM public.t;\n",
                encoding="utf-8",
            )

            code, out = run_tool(root)
            self.assertEqual(code, 0, f"legacy 应被排除，期望 exit 0, 实测 {code}\n{out}")


if __name__ == "__main__":
    unittest.main(verbosity=2)
