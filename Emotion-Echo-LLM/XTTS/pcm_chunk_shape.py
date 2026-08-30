"""PCM chunk shape conversion: float32 → int16 with volume + clip.

Extracted from the streaming loop in server.py per Stage 26-T
backlog §五 5.2.24 (pcm_chunk_shape.py ~100 LOC). Before this commit
the three operations were inlined in the streamer loop and not unit-
testable in isolation.

The function is intentionally pure numpy — no torch dependency, so
tests can import this module without triggering XTTS model loading.
"""
from __future__ import annotations

import numpy as np


def pcm_chunk_shape(chunk: np.ndarray, volume: float) -> np.ndarray:
    """Apply volume scaling + clipping + dtype conversion to a chunk.

    Args:
        chunk: float32 ndarray in [-1.0, 1.0] (audio samples).
        volume: linear gain multiplier (0 = mute, 1 = pass-through,
            2 = boost 6 dB).

    Returns:
        int16 ndarray of the same shape as `chunk`, with values in
        [-32767, 32767].

    Pipeline (matches the original inline code in server.py):
        1. scale by volume → may exceed [-1, 1]
        2. clip to [-1.0, 1.0] → int16 safe range
        3. (chunk * 32767).astype(np.int16)
    """
    scaled = chunk * volume
    clipped = np.clip(scaled, -1.0, 1.0)
    return (clipped * 32767).astype(np.int16)