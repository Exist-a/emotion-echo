"""
test_pcm_chunk_shape.py · XTTS PCM chunk shape 转换单元测试

Stage 26-T backlog §五 5.2.24: 覆盖 XTTS streaming 的 float32 → int16
转换契约（volume / clip / dtype）。

设计：pcm_chunk_shape 是 server.py streaming 循环里 inline 的数学
操作。这条 backlog 要求抽出独立可测的函数，本 commit 通过新增
pcm_chunk_shape.py 模块满足，并把 server.py streaming 循环改成调用
该模块（RED 先行 → GREEN）。

契约（per backlog §五 5.2.24）：
  - float32 input → int16 output:  (chunk * 32767).astype(np.int16)
  - np.clip(-1.0, 1.0) 边界保护（volume>1 时不能爆 int16）
  - volume=0 → 全静音 (全 0)
  - volume=2.0 → 不超 int16 上界 32767
"""
from __future__ import annotations

import sys
from pathlib import Path

import numpy as np

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from pcm_chunk_shape import pcm_chunk_shape  # noqa: E402


class TestVolumeTypeDConversion:
    def test_output_dtype_is_int16(self):
        chunk = np.array([0.0, 0.5, -0.5, 1.0, -1.0], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        assert out.dtype == np.int16

    def test_silence_at_zero_volume(self):
        chunk = np.array([0.1, -0.5, 0.99, -0.99], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=0.0)
        assert np.all(out == 0)

    def test_zero_audio_stays_zero(self):
        chunk = np.zeros(8, dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        assert np.all(out == 0)

    def test_half_scale_maps_to_half_int16(self):
        # 0.5 → 0.5 * 32767 ≈ 16383 (or 16384, depends on rounding)
        chunk = np.array([0.5], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        # int16 mapping: round(0.5 * 32767) = 16384 (or 16383)
        assert 16380 <= out[0] <= 16387

    def test_negative_half_scale(self):
        chunk = np.array([-0.5], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        assert -16387 <= out[0] <= -16380

    def test_full_scale_positive_clamps_to_int16_max(self):
        chunk = np.array([1.0], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        # 1.0 * 32767 = 32767, but per common TTS impls it clamps at 32767
        # not 32768 (int16 max).
        assert out[0] == 32767

    def test_full_scale_negative_clamps_to_int16_min(self):
        chunk = np.array([-1.0], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        assert out[0] == -32767

    def test_volume_above_one_does_not_exceed_int16_max(self):
        # volume=2.0 on a 0.99 sample → 1.98 → clip → 1.0 → 32767
        chunk = np.array([0.99], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=2.0)
        assert out[0] == 32767

    def test_volume_above_one_does_not_exceed_int16_min(self):
        chunk = np.array([-0.99], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=2.0)
        assert out[0] == -32767

    def test_overshoot_input_clamped_to_one(self):
        # If somehow input > 1.0 (shouldn't happen but defensive)
        chunk = np.array([2.0, -2.0], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        assert out[0] == 32767
        assert out[1] == -32767

    def test_volume_below_one_quietens_signal(self):
        # volume=0.5 of full scale should be ≈ 16384
        chunk = np.array([1.0], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=0.5)
        # 0.5 * 32767 = 16383.5 → 16383 or 16384
        assert 16380 <= out[0] <= 16387

    def test_shape_preserved(self):
        chunk = np.zeros((2, 4), dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        assert out.shape == (2, 4)
        assert out.dtype == np.int16

    def test_returns_new_array_not_view(self):
        # 修改输出不应影响输入（避免上游意外副作用）。
        chunk = np.array([0.5], dtype=np.float32)
        out = pcm_chunk_shape(chunk, volume=1.0)
        out[0] = -32768
        assert chunk[0] == 0.5  # input unchanged