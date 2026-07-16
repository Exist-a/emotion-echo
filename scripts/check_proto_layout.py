#!/usr/bin/env python3
"""
check_proto_layout.py · Emotion-Echo proto 文件布局自检

校验项目：
  1. 仓库根目录**不存在**散落的 .pb.go 文件（应全部在 emotion-echo-shared/pkg/）
  2. 仓库根目录**不存在**散落的 _grpc.pb.go 文件
  3. proto/ 目录包含至少一个 .proto 源文件
  4. emotion-echo-shared/pkg/emotionllm/ 包含 emotion_llm.pb.go（保留版）
  5. emotion-echo-shared/pkg/emotionquery/ 包含 emotion_query 相关 pb.go（保留版）

设计意图：
  proto 生成物（pb.go）是 source-of-truth 衍生品，应该全部归位到
  emotion-echo-shared/pkg/{emotionllm,emotionquery}/，根目录散落是历史遗留。

用法：
  python scripts/check_proto_layout.py            # 输出检查结果
  python scripts/check_proto_layout.py --strict   # 任一失败 exit 1
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# 仓库根目录散落的 .pb.go（这些是被规范化掉的）
SCATTERED_PB_PATTERNS = ["*.pb.go", "*_grpc.pb.go"]

# 必须存在的目标位置（source of truth）
TARGET_LOCATIONS = [
    ("emotion-echo-shared/pkg/emotionllm/emotion_llm.pb.go", "EmotionLLM Go pb"),
    ("emotion-echo-shared/pkg/emotionllm/emotion_llm_grpc.pb.go", "EmotionLLM Go gRPC stub"),
    ("emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go", "EmotionQuery Go pb"),
]


def check(label: str, ok: bool, detail: str = "") -> bool:
    mark = "✅" if ok else "❌"
    print(f"  {mark} {label}" + (f"  ({detail})" if detail else ""))
    return ok


def main() -> int:
    print("=" * 60)
    print("Emotion-Echo · proto 文件布局自检")
    print("=" * 60)
    print(f"ROOT = {ROOT}")
    print()

    all_ok = True

    # 1. 根目录无散落 .pb.go
    print("[1/3] 根目录无散落 pb 文件")
    scattered = []
    for pattern in SCATTERED_PB_PATTERNS:
        for p in ROOT.glob(pattern):
            # 只检查根目录的直接子项，不递归（避免误伤 .git/ 等）
            if p.parent == ROOT:
                scattered.append(p)
    if scattered:
        for p in scattered:
            all_ok &= check(f"  散落文件应删除: {p.name}", False, str(p))
    else:
        all_ok &= check("  根目录无散落 .pb.go", True)

    # 2. proto/ 目录有 .proto 源文件
    print("\n[2/3] proto 源文件存在")
    proto_dir = ROOT / "proto"
    if proto_dir.is_dir():
        proto_files = list(proto_dir.glob("*.proto"))
        all_ok &= check(
            f"  proto/ 包含 {len(proto_files)} 个 .proto 文件",
            len(proto_files) > 0,
            ", ".join(p.name for p in proto_files),
        )
    else:
        all_ok &= check("proto/ 目录存在", False, str(proto_dir))

    # 3. 目标位置的 pb.go 存在
    print("\n[3/3] 目标位置 pb.go 完整")
    for rel_path, desc in TARGET_LOCATIONS:
        target = ROOT / rel_path
        all_ok &= check(f"  {desc} 存在", target.is_file(), rel_path)

    # 汇总
    print()
    print("=" * 60)
    if all_ok:
        print("✅ 所有检查通过")
        return 0
    else:
        print("❌ 部分检查未通过，请根据上面 ❌ 项修正")
        if "--strict" in sys.argv:
            return 1
        return 0


if __name__ == "__main__":
    sys.exit(main())