# Stage 36-D xtts fix: torchaudio 2.11 hard-codes torchcodec backend.
# soundfile is already installed in the image, so monkey-patch torchaudio.load + save
# to delegate to it. Resampling and other torchaudio.functional.* paths are
# unaffected (they don't depend on torchcodec).
import io
import soundfile
import torch
import torchaudio  # noqa: F401

_orig_load = torchaudio.load
_orig_save = torchaudio.save


def _load_with_soundfile(filepath, *args, **kwargs):
    data, sr = soundfile.read(filepath)
    tensor = torch.from_numpy(data)
    if tensor.ndim == 1:
        tensor = tensor.unsqueeze(0)
    tensor = tensor.float()  # Stage 36-D TTS dtype fix: XTTS conv1d weight is float32
    return tensor, sr


def _save_with_soundfile(filepath, src, sample_rate, *args, **kwargs):
    """soundfile.write PCM WAV. Accepts torchaudio.save(buf, tensor, sr, format='wav') signature."""
    # src shape is (channels, samples); convert to numpy
    if hasattr(src, "detach"):
        arr = src.detach().cpu().numpy()
    else:
        arr = src
    if arr.ndim == 2:
        arr = arr.squeeze(0)
    # Determine format from kwargs or filepath suffix
    fmt = kwargs.get("format") or "wav"
    subtype = kwargs.get("encoding") or "PCM_16"
    if hasattr(filepath, "write"):
        # BytesIO-like
        soundfile.write(filepath, arr, sample_rate, format=fmt, subtype=subtype)
    else:
        soundfile.write(filepath, arr, sample_rate, format=fmt, subtype=subtype)


torchaudio.load = _load_with_soundfile
torchaudio.save = _save_with_soundfile