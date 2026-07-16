import { ref, onUnmounted } from "vue";
import { stripMarkdown, extractReadableText } from "~/utils/stripMarkdown";
import PcmPlayer from "pcm-player";

export type LipShape = 'aa' | 'ee' | 'ih' | 'oh' | 'ou' | 'neutral';

export interface Phoneme {
  char: string;
  start: number;
  duration: number;
}

export interface TTSRequest {
  text: string;
  language?: string;
}

export interface TTSResponse {
  audio: string;
  sample_rate: number;
  text?: string;
  phonemes?: Phoneme[];
  duration?: number;
}

export interface LipSyncItem {
  text: string;
  phonemes: Phoneme[];
  audio: HTMLAudioElement;
  startTime: number;
}

type LipSyncCallback = (shape: LipShape, progress: number) => void;

const VOWEL_TO_LIP: Record<string, LipShape> = {
  'a': 'aa',
  'o': 'oh',
  'e': 'ee',
  'i': 'ih',
  'u': 'ou',
  'ü': 'ee',
  'v': 'ih',
  'n': 'ih',
  'm': 'ih',
};

const CONSONANT_CLOSE: Record<string, LipShape> = {
  'b': 'aa',
  'p': 'aa',
  'm': 'aa',
  'f': 'oh',
  'v': 'oh',
  'w': 'ou',
  'd': 'ih',
  't': 'ih',
  'n': 'ih',
  'l': 'ih',
  'z': 'ih',
  'c': 'ih',
  's': 'ih',
  'zh': 'ih',
  'ch': 'ih',
  'sh': 'ih',
  'r': 'ih',
  'j': 'ih',
  'q': 'ih',
  'x': 'ih',
  'y': 'ih',
  'g': 'ih',
  'k': 'ih',
  'h': 'ih',
};

// 单例模式的变量
let audioContext: AudioContext | null = null;
let audioElement: HTMLAudioElement | null = null;
let currentTime = ref(0);
let isPlaying = ref(false);
let currentVolume = ref(2.0);
let pcmPlayer: any = null;
let lipAnimationInterval: ReturnType<typeof setInterval> | null = null;
let lipSyncCallback: LipSyncCallback | null = null;
let abortController: AbortController | null = null;
let chunksBuffer: Uint8Array[] = [];

const lipShapes: LipShape[] = ['aa', 'ee', 'ih', 'oh', 'ou'];

const startRandomLipAnimation = (callback?: LipSyncCallback) => {
  stopLipAnimation();
  lipSyncCallback = callback;

  let shapeIndex = 0;
  const interval = 150;

  lipAnimationInterval = setInterval(() => {
    const shape = lipShapes[shapeIndex % lipShapes.length];
    callback?.(shape, 1);
    shapeIndex++;
  }, interval);
};

const stopLipAnimation = () => {
  if (lipAnimationInterval) {
    clearInterval(lipAnimationInterval);
    lipAnimationInterval = null;
    lipSyncCallback?.('neutral', 0);
  }
};

const stop = () => {
  clearLipSyncInterval();
  stopLipAnimation();

  if (audioElement) {
    audioElement.pause();
    audioElement.currentTime = 0;
  }

  if (pcmPlayer) {
    if (typeof pcmPlayer.stop === 'function') {
      pcmPlayer.stop();
    }
    if (typeof pcmPlayer.destroy === 'function') {
      pcmPlayer.destroy();
    }
    pcmPlayer = null;
  }

  if (abortController) {
    abortController.abort();
    abortController = null;
  }

  chunksBuffer = [];
  isPlaying.value = false;
};

const clearLipSyncInterval = () => {
  if (lipAnimationInterval) {
    clearInterval(lipAnimationInterval);
    lipAnimationInterval = null;
  }
};

const setVolume = (volume: number) => {
  console.log('[TTS] setVolume called:', volume, 'pcmPlayer exists:', !!pcmPlayer, 'currentVolume:', currentVolume.value);
  currentVolume.value = volume;
  if (pcmPlayer) {
    pcmPlayer.volume = volume;
    console.log('[TTS] pcmPlayer.volume set to:', pcmPlayer.volume);
  }
};

const playStream = async (
  text: string,
  onLipSync: LipSyncCallback,
  speed: number = 0.75,
  volume: number = 2.0
) => {
  const cleanText = stripMarkdown(text).trim();
  if (!cleanText) return;

  const readableText = extractReadableText(cleanText);
  if (!readableText) return;

  console.log('[TTS Stream] Playing:', readableText.substring(0, 50));
  console.log('[TTS Stream] Starting new TTS stream, stopping previous...');
  
  stop();

  try {
    abortController = new AbortController();
    const response = await fetch('http://localhost:8003/tts_stream', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        text: readableText,
        speed: speed,
        volume: volume,
      }),
      signal: abortController.signal,
    });

    if (!response.ok) {
      throw new Error(`TTS stream request failed: ${response.status}`);
    }

    if (!response.body) {
      throw new Error("No response body");
    }

    const player = new PcmPlayer({
      inputCodec: 'Int16',
      channels: 1,
      sampleRate: 24000,
      flushTime: 100,
    });

    player.volume = currentVolume.value;
    pcmPlayer = player;
    console.log("[TTS Stream] PCM Player created with volume:", currentVolume.value);

    isPlaying.value = true;
    let lipAnimationStarted = false;

    const reader = response.body.getReader();

    let chunkCount = 0;
    let totalBytes = 0;
    const startTime = Date.now();
    let lastLogTime = startTime;

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      if (abortController?.signal.aborted) {
        console.log("[TTS Stream] Aborted");
        break;
      }

      if (value && value.length > 0) {
        const now = Date.now();
        chunkCount++;
        totalBytes += value.length;

        if (now - lastLogTime >= 500) {
          console.log(`[TTS Stream] Chunk #${chunkCount}, ${value.length} bytes, total: ${totalBytes} bytes, elapsed: ${(now - startTime) / 1000}s`);
          lastLogTime = now;
        }

        if (!lipAnimationStarted && chunkCount === 1) {
          console.log("[TTS Stream] First audio chunk received, starting lip animation...");
          startRandomLipAnimation(onLipSync);
          lipAnimationStarted = true;
        }

        try {
          let dataToUse = value;
          // 如果是奇数长度，去掉最后一个字节保证对齐
          if (value.length % 2 !== 0) {
            const newLength = value.length - 1;
            dataToUse = new Uint8Array(newLength);
            dataToUse.set(value.subarray(0, newLength));
          }
          const int16Data = new Int16Array(dataToUse.buffer);
          player.feed(int16Data);
        } catch (e) {
          console.error("[TTS Stream] Error feeding data to PCM player:", e);
        }
      }
    }

    console.log(`[TTS Stream] Stream completed, total chunks: ${chunkCount}, total bytes: ${totalBytes}, total time: ${(Date.now() - startTime) / 1000}s`);
    isPlaying.value = false;
    stopLipAnimation();

  } catch (e: any) {
    if (e.name === 'AbortError') {
      console.log("[TTS Stream] Request cancelled");
    } else {
      console.error("[TTS Stream] Error:", e);
      stopLipAnimation();
      throw e;
    }
  }
};

const playStreamChunks = (chunks: Uint8Array[]) => {
  if (!audioContext) return;

  const allBytes = new Uint8Array(chunks.reduce((acc, chunk) => acc + chunk.length, 0));
  let offset = 0;
  for (const chunk of chunks) {
    allBytes.set(chunk, offset);
    offset += chunk.length;
  }

  audioContext.decodeAudioData(allBytes.buffer, (buffer) => {
    const source = audioContext!.createBufferSource();
    source.buffer = buffer;
    source.connect(audioContext!.destination);
    source.start();
  }, (e) => {
    console.error("[TTS Stream] Decode error:", e);
  });
};

const pause = () => {
  if (audioElement && isPlaying.value) {
    audioElement.pause();
  }
};

const resume = () => {
  if (audioElement && !isPlaying.value && currentTime.value > 0) {
    audioElement.play();
  }
};

const flushBuffer = async (onLipSync: LipSyncCallback) => {
  if (chunksBuffer.length > 0) {
    console.log(`[TTS] Flushing buffer: ${chunksBuffer.length} chunks`);
    playStreamChunks([...chunksBuffer]);
    chunksBuffer = [];
  }
};

export function useTTSPlayer() {
  onUnmounted(() => {
    // 组件卸载时不停止播放器，因为是单例
  });

  return {
    currentTime,
    isPlaying,
    playStream,
    stop,
    pause,
    resume,
    flushBuffer,
    setVolume,
    clearLipSyncInterval,
  };
}
