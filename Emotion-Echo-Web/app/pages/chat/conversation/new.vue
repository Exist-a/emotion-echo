<template>
  <div class="new-conversation">
    <h1 class="title">你好啊，让我们开始聊天吧</h1>
    <div class="sender-container">
      <el-input
        v-model="message"
        type="textarea"
        :rows="3"
        :maxlength="2000"
        resize="none"
        placeholder="请输入想倾诉的内容，按发送按钮提交..."
        class="sender-input"
        @keydown.enter.prevent="handleEnterSubmit"
      />
      <div class="sender-actions">
        <div class="actions-left">
          <el-button
            :icon="Paperclip"
            circle
            size="small"
            class="action-btn"
            @click="handleAttachment"
          />
        </div>
        <div class="actions-right">
          <div
            class="voice-record-btn"
            :class="{ recording: isRecording }"
            @click="toggleRecording"
          >
            <div v-if="!isRecording" class="microphone-icon">
              <el-icon>
                <Microphone />
              </el-icon>
            </div>
            <div v-else class="voice-center-dot"></div>
            <div v-if="isRecording" class="voice-ring"></div>
          </div>
          <el-divider
            direction="vertical"
            border-style="dashed"
            class="vertical-divider"
          />
          <el-button
            circle
            :icon="Promotion"
            type="primary"
            size="small"
            class="send-btn"
            :disabled="!message.trim()"
            @click="handleSubmit"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { watch } from "vue";
import { Promotion, Microphone, Paperclip } from "@element-plus/icons-vue";
import { ElNotification } from "element-plus";
import { useDigitalHumanTTS } from "~/composables/useDigitalHumanTTS";
import { useDigitalHumanStore } from "~/stores/digitalHuman";
import { post } from "~/composables/useApi";

const digitalHumanStore = useDigitalHumanStore();
const { playText, flushRemaining, stop } = useDigitalHumanTTS({
  onLipShapeChange: (shape) => {
    digitalHumanStore.setLipShape(shape);
  }
});

// 监听静音状态变化，静音时立即停止播放
watch(
  () => digitalHumanStore.voiceEnabled,
  (newVal) => {
    if (!newVal) {
      console.log('[NEW] Muted, stopping TTS')
      stop()
    }
  }
);
const { createNewConversation } = useConversationSender();
const message = ref("");
const isRecording = ref(false);
const isUploading = ref(false);
let accumulatedDeltaText = "";
let ttsDebounceTimer: ReturnType<typeof setTimeout> | null = null;
let mediaRecorder: MediaRecorder | null = null;
let audioChunks: Blob[] = [];
let timer: number | null = null;
let startTime = 0;

const handleSubmit = async () => {
  const value = message.value;
  if (!value.trim()) return;

  accumulatedDeltaText = "";
  if (ttsDebounceTimer) {
    clearTimeout(ttsDebounceTimer);
    ttsDebounceTimer = null;
  }
  stop();

  await createNewConversation(value.trim(), {
    onDelta: (delta) => {
      accumulatedDeltaText += delta;

      if (ttsDebounceTimer) {
        clearTimeout(ttsDebounceTimer);
      }
      ttsDebounceTimer = setTimeout(() => {
        if (accumulatedDeltaText.trim().length > 0) {
          playText(accumulatedDeltaText);
          accumulatedDeltaText = "";
        }
      }, 500);
    },
    onFinish: () => {
      if (ttsDebounceTimer) {
        clearTimeout(ttsDebounceTimer);
        ttsDebounceTimer = null;
      }

      if (accumulatedDeltaText.trim().length > 0) {
        playText(accumulatedDeltaText);
        accumulatedDeltaText = "";
      }

      flushRemaining();
    }
  });
  message.value = "";
};

const handleEnterSubmit = (e: KeyboardEvent) => {
  if (!e.shiftKey) {
    handleSubmit();
  }
};

const handleAttachment = () => {
  console.log("打开附件上传");
};

const toggleRecording = async () => {
  if (isRecording.value) {
    stopRecording();
  } else {
    await startRecording();
  }
};

const startRecording = async () => {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });

    audioChunks = [];
    mediaRecorder = new MediaRecorder(stream, {
      mimeType: "audio/webm;codecs=opus"
    });

    mediaRecorder.ondataavailable = (event) => {
      if (event.data.size > 0) {
        audioChunks.push(event.data);
      }
    };

    mediaRecorder.onstop = async () => {
      const audioBlob = new Blob(audioChunks, { type: "audio/webm" });
      const duration = Math.round((Date.now() - startTime) / 1000);

      stream.getTracks().forEach((track) => track.stop());

      isUploading.value = true;
      try {
        const formData = new FormData();
        formData.append("file", audioBlob, "recording.webm");

        const result = await post("/voice/upload", formData);

        if (result) {
          const { messageId, transcript, emotion, audioUrl } = result as any;

          const userMessage: any = {
            id: messageId,
            sender: "user",
            content: transcript || "",
            contentType: "audio",
            audioUrl: audioUrl || "",
            audioDuration: duration,
            emotionTag: emotion,
            status: "sent",
            sendTime: Date.now()
          };

          accumulatedDeltaText = "";
          if (ttsDebounceTimer) {
            clearTimeout(ttsDebounceTimer);
            ttsDebounceTimer = null;
          }
          stop();

          await createNewConversation(transcript || "", {
            onDelta: (delta) => {
              accumulatedDeltaText += delta;

              if (ttsDebounceTimer) {
                clearTimeout(ttsDebounceTimer);
              }
              ttsDebounceTimer = setTimeout(() => {
                if (accumulatedDeltaText.trim().length > 0) {
                  playText(accumulatedDeltaText);
                  accumulatedDeltaText = "";
                }
              }, 500);
            },
            onFinish: () => {
              if (ttsDebounceTimer) {
                clearTimeout(ttsDebounceTimer);
                ttsDebounceTimer = null;
              }

              if (accumulatedDeltaText.trim().length > 0) {
                playText(accumulatedDeltaText);
                accumulatedDeltaText = "";
              }

              flushRemaining();
            },
            userMessage: userMessage
          });
        }
      } catch (error) {
        console.error("[Voice] Upload error:", error);
        ElNotification({
          title: "语音上传失败",
          message: "请重试",
          type: "error"
        });
      } finally {
        isUploading.value = false;
      }
    };

    mediaRecorder.start();
    isRecording.value = true;
    startTime = Date.now();

    timer = window.setInterval(() => {
      const elapsed = Math.round((Date.now() - startTime) / 1000);
    }, 1000);
  } catch (error) {
    console.error("[Voice] Recording error:", error);
    ElNotification({
      title: "语音录制失败",
      message: "请检查麦克风权限",
      type: "error"
    });
  }
};

const stopRecording = () => {
  if (mediaRecorder && mediaRecorder.state !== "inactive") {
    mediaRecorder.stop();
  }

  if (timer) {
    clearInterval(timer);
    timer = null;
  }

  isRecording.value = false;
};
</script>

<style scoped lang="scss">
.new-conversation {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;

  .title {
    text-align: center;
    font-size: clamp(18px, 4vw, 40px);
    margin: 15vh 0;
    font-weight: 400;
  }
}

.sender-container {
  width: clamp(300px, 80%, 1200px);
  max-width: 100%;
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: $radius-xxl;
  background-color: transparent;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;

  .sender-input {
    :deep(.el-textarea__inner) {
      border: none;
      box-shadow: none;
      background-color: transparent;
      resize: none;
      padding: 8px 4px;

      &::placeholder {
        color: #9ca3af;
        font-size: 14px;
      }
    }
  }
}

.sender-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 4px 0;

  .actions-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .actions-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .action-btn {
    color: #6b7280;
    border-color: #e5e7eb;
    &:hover {
      color: #409eff;
      border-color: #c6e2ff;
      background-color: #f0f9ff;
    }
  }

  .vertical-divider {
    height: 24px !important;
    margin: 0 6px;
    border-color: #e5e7eb;
  }

  .send-btn {
    background-color: #409eff;
    border-color: #409eff;
    width: 36px;
    height: 36px;
    &:hover {
      background-color: #66b1ff;
      border-color: #66b1ff;
    }
    &:disabled {
      background-color: #a0cfff;
      border-color: #a0cfff;
      cursor: not-allowed;
    }
  }

  .voice-record-btn {
    width: 36px;
    height: 36px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    position: relative;
    background-color: #f5f5f5;
    transition: all 0.3s;

    &:hover {
      background-color: #e9d5ff;
    }

    .microphone-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      color: #3388ea;
      font-size: 18px;
    }

    .voice-center-dot {
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background-color: #f56c6c;
      z-index: 2;
      transition: all 0.3s;
    }

    .voice-ring {
      position: absolute;
      border-radius: 50%;
      border: 2px solid #f56c6c;
      animation: ring-pulse 1.5s infinite ease-out;
      z-index: 1;
      width: 36px;
      height: 36px;
    }

    &.recording {
      background-color: #fff;

      .voice-center-dot {
        transform: scale(1.2);
      }
    }
  }
}

@keyframes ring-pulse {
  0% {
    transform: scale(0.8);
    opacity: 1;
  }
  100% {
    transform: scale(1.6);
    opacity: 0;
  }
}

@media (max-width: 768px) {
  .sender-container {
    width: 95%;
  }
}
</style>