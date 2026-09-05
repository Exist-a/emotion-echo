"""test_server_tts_dtype.py · XTTS /tts 输出张量 dtype 契约测试

Stage 36-FU Bug 3 follow-up:
  即使 torchaudio_shim 已经在 load 端强制 .float(), XTTS /tts 端点内部
  仍然用 `torch.tensor(audio).unsqueeze(0)` 直接构造张量。如果 `audio`
  是 numpy float64 (Coqui TTS speaker_encoder 重 load 的 wav 默认就是
  float64), 写 wav 时 torchaudio.save 不会转 dtype, 但若后续路径走
  conv1d 权重 (float32) 就会触发:

      RuntimeError: expected scalar type Double but found Float

  契约: server.py 在调用 `torchaudio.save` 之前, 把 tensor 显式转 float32。
  AST 检查所有 `torchaudio.save(...)` 调用, 第一个 tensor 参数的构造链上
  必须包含 `.float()` 调用 (无论从 `torch.tensor(...)` 还是 `torch.from_numpy(...)` 开始)。

这个测试在 dev / CI 都能跑 (不依赖 torch), 行为契约护栏;
与 test_torchaudio_shim.py 互补 (load 端 + save 端都保护)。
"""
from __future__ import annotations

import ast
import sys
from pathlib import Path

SERVICE_DIR = Path(__file__).resolve().parents[2]
SERVER_PATH = SERVICE_DIR / "server.py"


def _all_function_bodies(tree: ast.AST) -> list[ast.FunctionDef | ast.AsyncFunctionDef]:
    return [n for n in ast.walk(tree) if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))]


def _torchaudio_save_call_sites(tree: ast.AST) -> list[ast.Call]:
    """所有 torchaudio.save(...) 调用点。"""
    sites: list[ast.Call] = []
    for node in ast.walk(tree):
        if (
            isinstance(node, ast.Call)
            and isinstance(node.func, ast.Attribute)
            and node.func.attr == "save"
        ):
            # 形如 `torchaudio.save(...)` 或 `torchaudio_shim.load(...)` 之类。
            base = node.func.value
            if isinstance(base, ast.Name) and base.id == "torchaudio":
                sites.append(node)
    return sites


def _tensor_construction_chain_has_float(call_arg: ast.AST) -> bool:
    """检查 `torchaudio.save(<tensor>, ...)` 中 <tensor> 表达式链上是否调过 .float()。

    接受的形态:
      - torch.tensor(audio).unsqueeze(0).float()
      - torch.tensor(audio).float().unsqueeze(0)
      - torch.from_numpy(arr).float()
      - 任何 .float() 调用在调用链上即可
    """
    # 只接受形状合法的 shape-mutating / no-op 链: torch.tensor(x).<chain>.float()
    # 拒绝形如 foo(bar()).something(): 这种情况是复合表达式,我们无法在不执行的情况下
    # 判断 foo 是否保留了 dtype,因此要求最终必须显式调用 .float()。
    SAFE_ATTRS = {
        "float", "unsqueeze", "squeeze", "reshape", "view", "permute",
        "contiguous", "to", "cpu", "detach", "clone", "float32", "double",
        "half", "bfloat16", "type",
    }
    node = call_arg
    while True:
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute):
            if node.func.attr == "float":
                return True
            if node.func.attr in SAFE_ATTRS:
                node = node.func.value
                continue
            return False
        return False


def test_server_tts_endpoints_force_float32_before_save():
    """RED test for Stage 36-FU Bug 3 follow-up.

    Asserts: server.py 中所有 `torchaudio.save(...)` 调用的 tensor 参数,
    构造链上都包含 `.float()`。否则 /tts, /tts_with_phonemes 会因 numpy
    float64 audio 触发 conv1d dtype mismatch。
    """
    tree = ast.parse(SERVER_PATH.read_text(encoding="utf-8"))
    save_sites = _torchaudio_save_call_sites(tree)

    assert save_sites, "expected at least one torchaudio.save call in server.py"

    offenders: list[int] = []
    for site in save_sites:
        if len(site.args) < 2:
            continue
        # torchaudio.save signature: save(filepath, src, sample_rate, ...)
        # → src is positional arg index 1.
        tensor_arg = site.args[1]
        if not _tensor_construction_chain_has_float(tensor_arg):
            offenders.append(site.lineno)

    assert not offenders, (
        "Bug 3 regression: server.py lines "
        f"{offenders} call torchaudio.save without forcing .float() on the "
        "tensor. XTTS speaker_encoder may produce float64 numpy audio; "
        "if .float() is missing, downstream conv1d will raise "
        "'expected scalar type Double but found Float'."
    )


def test_server_tts_stream_endpoint_uses_pcm_chunk_shape():
    """RED test for streaming dtype safety.

    /tts_stream uses pcm_chunk_shape to do float→int16 conversion, which
    guards against dtype mismatch downstream. Verify this contract still
    holds so future refactors don't inline the math and lose the safety
    net.
    """
    tree = ast.parse(SERVER_PATH.read_text(encoding="utf-8"))

    # Find stream_audio_generator (the generator behind /tts_stream)
    stream_fn = next(
        (n for n in _all_function_bodies(tree) if n.name == "stream_audio_generator"),
        None,
    )
    assert stream_fn is not None, "server.py is missing stream_audio_generator"

    uses_pcm = False
    for node in ast.walk(stream_fn):
        if (
            isinstance(node, ast.Call)
            and isinstance(node.func, ast.Name)
            and node.func.id == "pcm_chunk_shape"
        ):
            uses_pcm = True
            break

    assert uses_pcm, (
        "stream_audio_generator must call pcm_chunk_shape(...) to keep the "
        "float→int16 conversion testable. Inlining the math here would "
        "silently regress the dtype-safety contract."
    )