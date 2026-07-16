# 语音模型服务部署指南

## 概述

本文档说明如何部署语音识别（ASR）和情绪识别所需的模型服务。

## 模型服务架构

```
┌─────────────────┐     ┌─────────────────┐
│   Qwen3-ASR     │     │   SenseVoice    │
│  语音转文字      │     │   情绪识别      │
│  localhost:8001  │     │  localhost:8002 │
└────────┬────────┘     └────────┬────────┘
         │                       │
         └───────────┬───────────┘
                     │
                     ▼
              ┌─────────────┐
              │  Go 后端    │
              │  :8081      │
              └─────────────┘
```

## 1. Qwen3-ASR 服务部署

### 环境要求

- Python 3.12+
- CUDA 11.7+ (推荐 CUDA 12.x)
- GPU 显存: 8GB+ (推荐 16GB+)

### 安装步骤

```bash
# 1. 创建虚拟环境
conda create -n qwen-asr python=3.12 -y
conda activate qwen-asr

# 2. 安装 qwen-asr 包
pip install -U qwen-asr

# 3. (可选) 安装 vLLM 加速
pip install -U "qwen-asr[vllm]"

# 4. (可选) 安装 FlashAttention
pip install -U flash-attn --no-build-isolation
```

### 启动服务

```bash
# 使用 transformers 后端（简单场景）
qwen-asr-serve Qwen/Qwen3-ASR-1.7B \
  --backend transformers \
  --cuda-visible-devices 0 \
  --host 0.0.0.0 \
  --port 8001

# 使用 vLLM 后端（高并发场景）
qwen-asr-serve Qwen/Qwen3-ASR-1.7B \
  --backend vllm \
  --gpu-memory-utilization 0.7 \
  --cuda-visible-devices 0 \
  --host 0.0.0.0 \
  --port 8001
```

### 验证服务

```bash
curl http://localhost:8001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "audio_url",
            "audio_url": {
              "url": "https://qianwen-res.oss-cn-beijing.aliyuncs.com/Qwen3-ASR-Repo/asr_zh.wav"
            }
          }
        ]
      }
    ]
  }'
```

## 2. SenseVoice 服务部署

### 环境要求

- Python 3.8+
- CUDA 11.0+ (推荐 CUDA 12.x)
- GPU 显存: 6GB+ (CPU 也可以运行，只是速度较慢)

### 支持的情绪标签

SenseVoice 支持以下情绪标签：
- `happy`：开心
- `sad`：悲伤
- `angry`：愤怒
- `neutral`：中性
- `unk`：未知

### 安装步骤

```bash
# 1. 创建虚拟环境
conda create -n sensevoice python=3.10 -y
conda activate sensevoice

# 2. 进入项目目录
cd Emotion-Echo/Emotion-Echo-LLM/sensevoice-small

# 3. 安装依赖
pip install -r requirements-server.txt
```

### 启动服务

项目已包含完整的服务脚本 `server.py`，支持多种启动方式：

```bash
# CPU 模式启动（默认）
python server.py --host 0.0.0.0 --port 8002 --device cpu

# GPU 模式启动
python server.py --host 0.0.0.0 --port 8002 --device cuda:0

# 后台运行（Linux/macOS）
nohup python server.py --host 0.0.0.0 --port 8002 > sensevoice.log 2>&1 &

# 后台运行（Windows PowerShell）
Start-Process python -ArgumentList "server.py","--host","0.0.0.0","--port","8002" -NoNewWindow
```

服务参数说明：
- `--host`：监听地址，默认 `0.0.0.0`
- `--port`：监听端口，默认 `8002`
- `--device`：运行设备，`cpu` 或 `cuda:0`，默认 `cpu`

### 验证服务

```bash
# 健康检查
curl http://localhost:8002/health

# 测试音频分析（使用示例音频）
curl -X POST http://localhost:8002/analyze \
  -F "file=@example/zh.mp3"
```

成功响应示例：
```json
{
  "emotion": "neutral",
  "confidence": 0.9,
  "text": "你好，这是一段测试语音。",
  "raw_text": "<|zh|><|NEUTRAL|><|Speech|>你好，这是一段测试语音。"
}
```

## 3. 配置后端

在 `configs/config.yaml` 中配置模型服务地址：

```yaml
ai:
  asr:
    enabled: true
    base_url: "http://localhost:8001"
    model: "Qwen/Qwen3-ASR-1.7B"
  emotion:
    enabled: true
    base_url: "http://localhost:8002"
```

## 4. 常见问题

### Q1: GPU 内存不足

```bash
# 减小 batch size
--backend-kwargs '{"gpu_memory_utilization": 0.5}'

# 或使用更小的模型
qwen-asr-serve Qwen/Qwen3-ASR-0.6B
```

### Q2: 音频格式不支持

推荐使用 WebM (Opus) 格式，前端录音默认就是这个格式。

如需转换，可使用 ffmpeg：

```bash
ffmpeg -i input.webm -acodec pcm_s16le -ar 16000 output.wav
```

### Q3: 服务启动失败

检查 CUDA 和 PyTorch 版本兼容性：

```bash
python -c "import torch; print(torch.cuda.is_available())"
```

## 5. 性能优化建议

1. **使用 vLLM 后端**：可提高吞吐量和降低延迟
2. **启用 FlashAttention**：可减少显存占用
3. **批量处理**：如果有多路并发请求，可考虑批量处理
4. **模型量化**：使用 INT8/INT4 量化减少显存占用

## 6. Docker 部署（可选）

### Qwen3-ASR

```bash
docker run --gpus all --name qwen-asr \
  -v /path/to/models:/data/models \
  -p 8001:8000 \
  --shm-size 4gb \
  -it qwenllm/qwen3-asr:latest
```

## 7. 下一步

服务部署完成后，返回后端开发，实现模型调用封装。
