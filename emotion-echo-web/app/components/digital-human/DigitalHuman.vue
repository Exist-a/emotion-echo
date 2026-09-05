<template>
  <div
    id="digital-human-wrapper"
    class="digital-human-wrapper"
    :class="{ draggable }"
    :style="{ left: position.x ? position.x + 'px' : '', top: position.y ? position.y + 'px' : '' }"
    @mousedown="handleMouseDown"
  >
    <div id="digital-human-container" class="digital-human-container" v-show="visible">
      <canvas ref="canvasRef" class="digital-human-canvas"></canvas>
      <div v-if="loading" class="loading-overlay">
        <div class="loading-spinner"></div>
        <span class="loading-text">加载数字人模型中...</span>
      </div>
      <div v-else-if="loadFailed" class="loading-overlay">
        <div class="error-icon">❌</div>
        <span class="loading-text">3D模型加载失败</span>
        <button class="retry-button" @click="retryLoadVRM">重试</button>
      </div>
    </div>
    <div class="control-buttons">
      <button
        class="control-btn"
        @click="handleToggleVisible"
        :title="visible ? '隐藏数字人' : '显示数字人'"
      >
        {{ visible ? '👁' : '🙈' }}
      </button>
      <button
        class="control-btn"
        @click="handleToggleVoice"
        :title="voiceEnabled ? '关闭语音' : '开启语音'"
      >
        {{ voiceEnabled ? '🔊' : '🔇' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader'
import { VRMLoaderPlugin } from '@pixiv/three-vrm'
import { useDigitalHumanStore } from '@/stores/digitalHuman'
import type { LipShape } from '~/composables/useTTSPlayer'

interface Props {
  modelPath?: string
  visible?: boolean
  draggable?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  modelPath: '/3d-models/digital-human.vrm',
  visible: true,
  draggable: true
})

interface Position {
  x: number
  y: number
}

const emit = defineEmits<{
  (e: 'position-change', position: Position): void
  (e: 'voice-toggle'): void
}>()

const digitalHumanStore = useDigitalHumanStore()
const canvasRef = ref<HTMLCanvasElement | null>(null)
const loading = ref(true)
const isDragging = ref(false)
const dragStartPos = { x: 0, y: 0 }
const elementStartPos = { x: 0, y: 0 }
const position = ref({ x: 0, y: 20 })
const showControls = ref(true)
const visible = ref(digitalHumanStore.visible)
const voiceEnabled = ref(digitalHumanStore.voiceEnabled)

// 同步store状态到本地
watch(
  () => digitalHumanStore.voiceEnabled,
  (newVal) => {
    voiceEnabled.value = newVal
  }
)

watch(
  () => digitalHumanStore.visible,
  (newVal) => {
    visible.value = newVal
  }
)

watch(
  () => digitalHumanStore.currentLipShape,
  (newShape) => {
    if (newShape && newShape !== 'neutral') {
      setLipShape(newShape as LipShape)
    }
  }
)

let scene: THREE.Scene | null = null
let camera: THREE.PerspectiveCamera | null = null
let renderer: THREE.WebGLRenderer | null = null
let animationFrameId: number | null = null
let vrmModel: any = null

let animationTime = 0
let lastBlinkTime = 0
const BLINK_INTERVAL = 5

let loadAttempts = 0
const MAX_LOAD_ATTEMPTS = 3
let loadFailed = ref(false)

let currentEmotion: string = 'neutral'
let currentLipShape: LipShape = 'neutral'
let emotionTransitionTime = 0
let lipShapeTransitionTime = 0
const EMOTION_DURATION = 5.0
const LIP_TRANSITION_SPEED = 20.0

const EMOTION_MAP: Record<string, string> = {
  happy: 'happy',
  sad: 'sad',
  angry: 'angry',
  anxious: 'neutral',
  neutral: 'neutral',
  unk: 'neutral',
  unknown: 'neutral'
}

const LIP_EMOTION_MAP: Record<LipShape, string> = {
  aa: 'neutral',
  ee: 'neutral',
  ih: 'neutral',
  oh: 'neutral',
  ou: 'neutral',
  neutral: 'neutral'
}

const setInitialPose = () => {
  if (!vrmModel || !vrmModel.humanoid) return

  try {
    // 设置手臂自然放下的姿势
    const leftUpperArm = vrmModel.humanoid.getRawBoneNode('leftUpperArm')
    const rightUpperArm = vrmModel.humanoid.getRawBoneNode('rightUpperArm')
    const leftLowerArm = vrmModel.humanoid.getRawBoneNode('leftLowerArm')
    const rightLowerArm = vrmModel.humanoid.getRawBoneNode('rightLowerArm')

    if (leftUpperArm) {
      leftUpperArm.rotation.x = 0.4
      leftUpperArm.rotation.z = 1.1
      leftUpperArm.rotation.y = 0.1
    }
    if (rightUpperArm) {
      rightUpperArm.rotation.x = 0.4
      rightUpperArm.rotation.z = -1.1
      rightUpperArm.rotation.y = -0.1
    }
    if (leftLowerArm) {
      leftLowerArm.rotation.x = 0.4
    }
    if (rightLowerArm) {
      rightLowerArm.rotation.x = 0.4
    }
  } catch (e) {
    console.warn('[DigitalHuman] Failed to set initial pose:', e)
  }
}

const updateAnimations = () => {
  if (!vrmModel || !vrmModel.humanoid) return

  const currentTime = Date.now() / 1000
  animationTime += 0.016

  try {
    const head = vrmModel.humanoid.getRawBoneNode('head')
    if (head) {
      head.rotation.y = Math.sin(animationTime * 0.4) * 0.08
      head.rotation.x = Math.sin(animationTime * 0.3) * 0.03
    }

    const spine = vrmModel.humanoid.getRawBoneNode('spine')
    if (spine) {
      spine.position.y = Math.sin(animationTime * 0.6) * 0.01
    }

    if (vrmModel.expressionManager) {
      const timeSinceLastBlink = currentTime - lastBlinkTime

      if (timeSinceLastBlink > BLINK_INTERVAL) {
        vrmModel.expressionManager.setValue('blink', 1.0)
        lastBlinkTime = currentTime
      } else if (timeSinceLastBlink > 0.15 && timeSinceLastBlink < 0.35) {
        vrmModel.expressionManager.setValue('blink', 0.0)
      }

      updateEmotionExpression(currentTime)
      updateLipShapeExpression(currentTime)

      vrmModel.expressionManager.update()
    }
  } catch (e) {
    console.warn('[DigitalHuman] Animation error:', e)
  }
}

const updateEmotionExpression = (currentTime: number) => {
  if (emotionTransitionTime > 0) {
    emotionTransitionTime -= 0.016
    if (emotionTransitionTime <= 0) {
      emotionTransitionTime = 0
      resetEmotionExpression()
    }
  }
}

const updateLipShapeExpression = (currentTime: number) => {
  const lipWeight = currentLipShape === 'neutral' ? 0.0 : 0.8

  const lipShapes: LipShape[] = ['aa', 'ee', 'ih', 'oh', 'ou']
  for (const shape of lipShapes) {
    const targetWeight = currentLipShape === shape ? lipWeight : 0.0
    try {
      vrmModel.expressionManager.setValue(shape, targetWeight)
    } catch (e) {}
  }
}

const resetEmotionExpression = () => {
  if (!vrmModel || !vrmModel.expressionManager) return

  try {
    vrmModel.expressionManager.setValue('happy', 0.0)
    vrmModel.expressionManager.setValue('sad', 0.0)
    vrmModel.expressionManager.setValue('angry', 0.0)
    vrmModel.expressionManager.setValue('relaxed', 0.0)
  } catch (e) {}
}

const setEmotion = (emotion: string) => {
  if (!vrmModel || !vrmModel.expressionManager) return

  const mappedEmotion = EMOTION_MAP[emotion] || 'neutral'
  if (mappedEmotion === currentEmotion) return

  console.log('[DigitalHuman] Setting emotion:', mappedEmotion)
  currentEmotion = mappedEmotion

  resetEmotionExpression()

  try {
    vrmModel.expressionManager.setValue(mappedEmotion, 1.0)
  } catch (e) {
    console.warn('[DigitalHuman] Failed to set emotion:', e)
  }

  emotionTransitionTime = EMOTION_DURATION
}

const setLipShape = (shape: LipShape) => {
  if (currentLipShape === shape) return

  currentLipShape = shape
}

const resetLipShape = () => {
  currentLipShape = 'neutral'
}

const initScene = () => {
  if (!canvasRef.value) {
    console.error('[DigitalHuman] Canvas ref is not available!')
    return
  }

  renderer = new THREE.WebGLRenderer({
    canvas: canvasRef.value,
    alpha: true,
    antialias: true
  })
  renderer.setPixelRatio(window.devicePixelRatio)
  renderer.setSize(canvasRef.value.clientWidth, canvasRef.value.clientHeight)

  scene = new THREE.Scene()

  camera = new THREE.PerspectiveCamera(
    45,
    canvasRef.value.clientWidth / canvasRef.value.clientHeight,
    0.1,
    1000
  )
  camera.position.set(0, 1.14, 0.7)
  camera.lookAt(0, 1.0, 0)

  const ambientLight = new THREE.AmbientLight(0xffffff, 1)
  scene.add(ambientLight)

  const directionalLight = new THREE.DirectionalLight(0xffffff, 1)
  directionalLight.position.set(1, 1, 1)
  scene.add(directionalLight)

  const animate = () => {
    animationFrameId = requestAnimationFrame(animate)
    updateAnimations()
    if (renderer && scene && camera) {
      renderer.render(scene, camera)
    }
  }
  animate()
}

const retryLoadVRM = async () => {
  loadAttempts = 0
  loadFailed.value = false
  loading.value = true
  await loadVRM()
}

const loadVRM = async () => {
  if (!scene) {
    console.error('[DigitalHuman] Scene is not initialized!')
    loadFailed.value = true
    loading.value = false
    return
  }

  const loader = new GLTFLoader()
  loader.register((parser) => new VRMLoaderPlugin(parser))

  try {
    loadAttempts++
    const model = await loader.loadAsync(props.modelPath)

    vrmModel = model.userData.vrm

    if (vrmModel && vrmModel.scene) {
      vrmModel.scene.position.set(0, -0.3, 0)
      vrmModel.scene.rotation.y = Math.PI
      scene.add(vrmModel.scene)
      setInitialPose()
    } else {
      console.warn('[DigitalHuman] Not a VRM model, adding as GLTF...')
      const modelScene = model.scene
      modelScene.position.set(0, -0.3, 0)
      modelScene.rotation.y = Math.PI
      scene.add(modelScene)
    }

    loading.value = false
    loadFailed.value = false
    // 初始化眨眼计时器
    lastBlinkTime = Date.now() / 1000
  } catch (error) {
    console.error('[DigitalHuman] Failed to load VRM model (Attempt ' + loadAttempts + '):', error)

    if (loadAttempts < MAX_LOAD_ATTEMPTS) {
      await new Promise((resolve) => setTimeout(resolve, 1000))
      await loadVRM()
    } else {
      loadFailed.value = true
      loading.value = false
    }
  }
}

const disposeThreeJS = () => {
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId)
    animationFrameId = null
  }

  if (vrmModel && vrmModel.scene && scene) {
    scene.remove(vrmModel.scene)
    vrmModel = null
  }

  if (renderer) {
    renderer.dispose()
    renderer = null
  }

  if (scene) {
    scene.traverse((object) => {
      if (object instanceof THREE.Mesh) {
        object.geometry?.dispose()
        if (Array.isArray(object.material)) {
          object.material.forEach((material) => material.dispose())
        } else {
          object.material?.dispose()
        }
      }
    })
    scene = null
  }

  camera = null
}

const handlePositionChange = () => {
  emit('position-change', { x: position.value.x, y: position.value.y })
}

const handleMouseDown = (e: MouseEvent) => {
  if (!props.draggable) return
  e.preventDefault()
  isDragging.value = true
  dragStartPos.x = e.clientX
  dragStartPos.y = e.clientY

  // 如果还没有拖动过，从CSS的初始位置开始计算
  if (position.value.x === 0 && position.value.y === 0) {
    // CSS是right: 20px，转换成left坐标
    elementStartPos.x = window.innerWidth - 60 - 20
    elementStartPos.y = 20
  } else {
    elementStartPos.x = position.value.x
    elementStartPos.y = position.value.y
  }

  document.addEventListener('mousemove', handleMouseMove)
  document.addEventListener('mouseup', handleMouseUp)
}

const handleMouseMove = (e: MouseEvent) => {
  if (!isDragging.value) return
  const deltaX = e.clientX - dragStartPos.x
  const deltaY = e.clientY - dragStartPos.y
  let newX = elementStartPos.x + deltaX
  let newY = elementStartPos.y + deltaY

  // 考虑圆形容器向左偏移了140px，加上wrapper本身
  const minX = -140
  const maxX = window.innerWidth - 60
  const maxY = window.innerHeight - 250

  newX = Math.max(minX, Math.min(newX, maxX))
  newY = Math.max(0, Math.min(newY, maxY))

  position.value.x = newX
  position.value.y = newY
  handlePositionChange()
}

const handleMouseUp = () => {
  isDragging.value = false
  document.removeEventListener('mousemove', handleMouseMove)
  document.removeEventListener('mouseup', handleMouseUp)
}

// const handleMouseEnter = () => {
//   showControls.value = true;
// };

// const handleMouseLeave = () => {
//   showControls.value = false;
// };

const handleToggleVisible = () => {
  digitalHumanStore.toggleVisible()
  visible.value = digitalHumanStore.visible
}

const handleToggleVoice = () => {
  emit('voice-toggle')
}

onMounted(() => {
  // 初始位置由CSS的fixed控制，这里设0让CSS接管
  position.value.x = 0
  position.value.y = 0
  initScene()
  loadVRM()
})

onUnmounted(() => {
  disposeThreeJS()
  document.removeEventListener('mousemove', handleMouseMove)
  document.removeEventListener('mouseup', handleMouseUp)
})

defineExpose({
  setEmotion,
  setLipShape,
  resetLipShape
})
</script>

<style scoped lang="scss">
#digital-human-wrapper {
  width: 60px; /* 宽一点放两个按钮 */
  height: 40px;
  position: fixed;
  right: 20px;
  top: 20px;
  z-index: 10000;

  &.draggable {
    cursor: move;
  }

  &:not(.draggable) {
    cursor: default;
  }
}

.digital-human-container {
  width: 200px;
  height: 200px;
  border-radius: 50%;
  background: white;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  border: 3px solid #fff;
  overflow: hidden;
  position: absolute;
  top: 0;
  left: 0;
}

.loading-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background-color: rgba(255, 255, 255, 0.9);
  z-index: 10;
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #f3f3f3;
  border-top: 3px solid #409eff;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}

.loading-text {
  margin-top: 12px;
  color: #606266;
  font-size: 14px;
}

.error-icon {
  font-size: 36px;
}

.retry-button {
  margin-top: 16px;
  padding: 8px 20px;
  border: none;
  border-radius: 20px;
  background-color: #409eff;
  color: white;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;

  &:hover {
    background-color: #66b1ff;
    transform: scale(1.05);
  }

  &:active {
    transform: scale(0.95);
  }
}

.digital-human-canvas {
  width: 100%;
  height: 100%;
  display: block;
}

.control-buttons {
  position: absolute;
  top: 0;
  right: 0;
  display: flex;
  gap: 6px;
  z-index: 2000;
}

.digital-human-container {
  width: 200px;
  height: 200px;
  border-radius: 50%;
  background: white;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  border: 3px solid #fff;
  overflow: hidden;
  position: absolute;
  top: 48px; /* 在按钮下方 */
  left: -140px; /* 向左偏移让圆形居中 */
}

.control-btn {
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 50%;
  background: rgba(64, 158, 255, 0.9);
  color: white;
  font-size: 14px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);

  &:hover {
    background: rgba(64, 158, 255, 1);
    transform: scale(1.1);
  }

  &:active {
    transform: scale(0.95);
  }
}
</style>
