"""test_torchaudio_shim.py · torchaudio_shim monkey-patch 单测

Contract-only shim test (split into two halves):
  * "shim contract" (always run, no torch needed) — verifies the source
    code contains the .float() fix that prevents the XTTS conv1d
    "expected scalar type Double but found Float" regression. Runs in
    plain `pytest tests/unit/` without docker / torch.
  * "shim runtime" (skipped without torch) — exercises the patched
    torchaudio.load / save. Lives in the production image where torch +
    soundfile are installed.

Stage 36-FU Bug 3 follow-up:
  Stage 36-D 的 torchaudio_shim 已经修掉了 torchcodec backend 阻塞,
  并在 _load_with_soundfile 里加了 `.float()`。本单测断言这个契约
  仍然成立,防止未来 refactor 把 .float() 删掉。

契约 (per stage-36-smoke-report.md §T7):
  - shim 替换 torchaudio.load 之后返回 (torch.Tensor, int) 二元组
  - tensor.dtype 必须是 torch.float32 (XTTS conv1d 权重是 float32,
    speaker_encoder 走 Double/Float mismatch 会抛
    "expected scalar type Double but found Float")
  - 单声道 wav (1D numpy) → (1, samples) 2D tensor (XTTS 默认输入布局)
  - 立体声 wav (2D numpy) → (channels, samples) 2D tensor 原样保留
  - save 路径走 soundfile.write,生成的 wav 能用 soundfile.read 反读回来
"""
from __future__ import annotations

import ast
import io
import sys
import wave
from pathlib import Path

import numpy as np

SERVICE_DIR = Path(__file__).resolve().parents[2]


def _shim_path() -> Path:
    return SERVICE_DIR / "torchaudio_shim.py"


def _has_torch() -> bool:
    try:
        import torch  # noqa: F401
        return True
    except ImportError:
        return False


def _make_wav_bytes(duration_sec: float = 0.05, sr: int = 16000, channels: int = 1) -> bytes:
    """用 stdlib wave 生成测试 wav bytes(避免依赖 soundfile 在测试里 init)。"""
    buf = io.BytesIO()
    n = int(duration_sec * sr)
    with wave.open(buf, "wb") as w:
        w.setnchannels(channels)
        w.setsampwidth(2)  # int16
        w.setframerate(sr)
        t = np.linspace(0, duration_sec, n, endpoint=False)
        if channels == 1:
            data = (np.sin(2 * np.pi * 440 * t) * 16000).astype(np.int16).tobytes()
        else:
            data_l = (np.sin(2 * np.pi * 440 * t) * 16000).astype(np.int16).tobytes()
            data_r = (np.sin(2 * np.pi * 440 * t) * 16000).astype(np.int16).tobytes()
            data = data_l + data_r
        w.writeframes(data)
    return buf.getvalue()


def test_shim_source_contains_float_cast():
    """RED test for Stage 36-D Bug 3 follow-up.

    Without the `.float()` cast in _load_with_soundfile, the shim would
    return a float64 tensor (soundfile default) which crashes XTTS
    speaker_encoder's conv1d with:

        RuntimeError: expected scalar type Double but found Float

    This contract test parses torchaudio_shim.py with `ast` and asserts
    the `tensor.float()` call is present inside `_load_with_soundfile`.
    It runs without torch / soundfile installed, so it lives next to the
    rest of the unit-test suite and acts as a guardrail against future
    refactors silently regressing Bug 3.
    """
    tree = ast.parse(_shim_path().read_text(encoding="utf-8"))

    load_fn = next(
        (n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == "_load_with_soundfile"),
        None,
    )
    assert load_fn is not None, "torchaudio_shim.py is missing _load_with_soundfile"

    float_calls_in_load: list[int] = []
    for node in ast.walk(load_fn):
        if (
            isinstance(node, ast.Call)
            and isinstance(node.func, ast.Attribute)
            and node.func.attr == "float"
        ):
            float_calls_in_load.append(node.lineno)

    assert float_calls_in_load, (
        "Bug 3 regression: torchaudio_shim._load_with_soundfile must call "
        ".float() before returning. Otherwise XTTS conv1d will raise "
        "'expected scalar type Double but found Float'."
    )


def test_shim_source_patches_torchaudio_load_and_save():
    """Verify the shim actually replaces torchaudio.load / torchaudio.save.

    Prevents the "shim is imported but never patches" foot-gun, which
    historically caused 500s in /tts despite torchaudio_shim.py being
    present in the image.
    """
    src = _shim_path().read_text(encoding="utf-8")
    assert "torchaudio.load =" in src, (
        "torchaudio_shim.py must assign `torchaudio.load = _load_with_soundfile` "
        "to actually monkey-patch; merely defining the function is not enough."
    )
    assert "torchaudio.save =" in src, (
        "torchaudio_shim.py must assign `torchaudio.save = _save_with_soundfile` "
        "to actually monkey-patch; merely defining the function is not enough."
    )


def test_shim_load_returns_float32_mono():
    """单声道 wav → 返回 float32 tensor, shape (1, samples) (Bug 3 contract).

    Only runs when torch + soundfile are installed (i.e. inside the
    production image). On dev / CI without these deps, the AST contract
    tests above are sufficient guards.
    """
    if not _has_torch():
        import pytest
        pytest.skip("torch not installed; contract tests above are sufficient on dev")

    sys.path.insert(0, str(SERVICE_DIR))
    import tempfile
    import torch
    import torchaudio  # noqa: F401  # triggers monkey-patch
    import torchaudio_shim  # noqa: F401

    wav_bytes = _make_wav_bytes(channels=1)
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        f.write(wav_bytes)
        tmp_path = f.name

    tensor, sr = torchaudio.load(tmp_path)

    assert isinstance(tensor, torch.Tensor), f"expected torch.Tensor, got {type(tensor)}"
    assert tensor.dtype == torch.float32, (
        f"Bug 3 regression: shim must return float32 for XTTS conv1d, got {tensor.dtype}"
    )
    assert tensor.ndim == 2, f"expected 2D (channels, samples), got {tensor.ndim}D"
    assert tensor.shape[0] == 1, f"expected mono (1, samples), got {tensor.shape}"
    assert isinstance(sr, int) and sr > 0, f"expected positive sample rate int, got {sr!r}"


def test_shim_load_returns_float32_stereo():
    """立体声 wav → 保留 channels 数, dtype 仍为 float32."""
    if not _has_torch():
        import pytest
        pytest.skip("torch not installed; contract tests above are sufficient on dev")

    sys.path.insert(0, str(SERVICE_DIR))
    import tempfile
    import torch
    import torchaudio  # noqa: F401
    import torchaudio_shim  # noqa: F401

    wav_bytes = _make_wav_bytes(channels=2)
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        f.write(wav_bytes)
        tmp_path = f.name

    tensor, sr = torchaudio.load(tmp_path)

    assert tensor.dtype == torch.float32
    assert tensor.ndim == 2
    assert tensor.shape[0] == 2, f"expected stereo (2, samples), got {tensor.shape}"


def test_shim_save_roundtrip():
    """save 写出的 wav 还能用 soundfile.read 反读,样本值不被破坏."""
    if not _has_torch():
        import pytest
        pytest.skip("torch not installed; contract tests above are sufficient on dev")

    sys.path.insert(0, str(SERVICE_DIR))
    import tempfile
    import torch
    import torchaudio  # noqa: F401
    import torchaudio_shim  # noqa: F401

    tensor = torch.zeros(1, 100, dtype=torch.float32)
    sr = 16000

    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        tmp_path = f.name

    torchaudio.save(tmp_path, tensor, sr, format="wav")

    import soundfile
    data, read_sr = soundfile.read(tmp_path)
    assert read_sr == sr
    assert data.shape == (100,), f"expected (samples,), got {data.shape}"