---
status: planned
superseded-by: 持续维护的工具手册
original-path: .trae/documents/three-vrm-usage-reference.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-C
---

# Three-VRM 模型控制参考文档

## 📚 概述
本文档记录如何使用 @pixiv/three-vrm 库来控制 VRM 模型的骨骼和表情。

---

## 🔍 核心API

### 1. 获取 VRM 实例
```typescript
const loader = new GLTFLoader();
loader.register((parser) => new VRMLoaderPlugin(parser));

const model = await loader.loadAsync(props.modelPath);
const vrmModel = model.userData.vrm;
```

---

### 2. 骨骼控制 (Humanoid)

#### 2.1 获取骨骼节点
```typescript
// 方法1：使用 getRawBoneNode
const headBone = vrmModel.humanoid.getRawBoneNode('head');

// 方法2：使用 getNormalizedBoneNode
const headBone = vrmModel.humanoid.getNormalizedBoneNode('head');

// 方法3：访问 rawHumanBones
const boneMap = vrmModel.humanoid.rawHumanBones;
const leftArm = boneMap.leftUpperArm;
```

#### 2.2 常见骨骼名称（小驼峰）
| 骨骼名称 | 说明 |
|---------|------|
| `hips` | 骨盆/臀部 |
| `spine` | 脊椎 |
| `neck` | 脖子 |
| `head` | 头部 |
| `leftUpperArm` | 左上臂 |
| `rightUpperArm` | 右上臂 |
| `leftLowerArm` | 左前臂 |
| `rightLowerArm` | 右前臂 |
| `leftUpperLeg` | 左大腿 |
| `rightUpperLeg` | 右大腿 |
| `leftLowerLeg` | 左小腿 |
| `rightLowerLeg` | 右小腿 |
| `leftHand` | 左手 |
| `rightHand` | 右手 |
| `leftShoulder` | 左肩 |
| `rightShoulder` | 右肩 |
| `upperChest` | 上胸 |
| `chest` | 胸 |

#### 2.3 打印可用骨骼
```typescript
console.log('All human bones:', Object.keys(vrmModel.humanoid.rawHumanBones));
```

#### 2.4 控制骨骼旋转
```typescript
// 获取右手臂骨骼
const rightArm = vrmModel.humanoid.getRawBoneNode('rightUpperArm');

if (rightArm) {
  // 旋转手臂 (THREE.Euler 或四元数)
  rightArm.rotation.x = 0.6;  // 向前
  rightArm.rotation.z = -0.6; // 向内
  rightArm.rotation.y = -0.1; // 内旋
}
```

#### 2.5 骨骼动画示例
```typescript
// 在动画循环中更新骨骼
let animationTime = 0;
const updateAnimations = () => {
  animationTime += 0.016;
  
  // 头部左右微动
  const head = vrmModel.humanoid.getRawBoneNode('head');
  if (head) {
    head.rotation.y = Math.sin(animationTime * 0.8) * 0.15; // 左右
    head.rotation.x = Math.sin(animationTime * 0.5) * 0.05; // 上下
  }
  
  // 身体呼吸感
  const spine = vrmModel.humanoid.getRawBoneNode('spine');
  if (spine) {
    spine.position.y = Math.sin(animationTime * 0.6) * 0.01;
  }
};
```

---

### 3. 表情控制 (BlendShape)

#### 3.1 打印可用表情
```typescript
// 打印所有信息
console.log('Expression Manager:', vrmModel.expressionManager);
console.log('Preset expressions:', Object.keys(vrmModel.expressionManager.presetExpressionMap));
console.log('Custom expressions:', Object.keys(vrmModel.expressionManager.customExpressionMap));
console.log('All expression names:', Array.from(vrmModel.expressionManager.expressions).map(expr => expr.expressionName));
```

#### 3.2 预设表情名称（小驼峰）
| 表情名称 | 说明 |
|---------|------|
| `neutral` | 中性 |
| `happy` | 开心 |
| `angry` | 生气 |
| `sad` | 悲伤 |
| `relaxed` | 放松 |
| `aa` | 啊（口型） |
| `ee` | 咿（口型） |
| `ih` | 一（口型） |
| `oh` | 哦（口型） |
| `ou` | 哦（口型） |
| `blink` | 眨眼 |
| `blinkLeft` | 左眼单眼 |
| `blinkRight` | 右眼单眼 |
| `lookUp` | 向上看 |
| `lookDown` | 向下看 |
| `lookLeft` | 向左看 |
| `lookRight` | 向右看 |

#### 3.3 自定义表情
- 自定义表情名称可能不是小驼峰！
- 例如：`Surprised` 是大写S开头

#### 3.4 控制表情权重
```typescript
// 设置表情权重 (0.0 ~ 1.0)
vrmModel.expressionManager.setValue('happy', 1.0);

// 混合多种表情
vrmModel.expressionManager.setValue('happy', 0.5);
vrmModel.expressionManager.setValue('Surprised', 0.3);

// 眨眼示例
let lastBlinkTime = 0;
const BLINK_INTERVAL = 5; // 秒
const currentTime = Date.now() / 1000;
const timeSinceLastBlink = currentTime - lastBlinkTime;

if (timeSinceLastBlink > BLINK_INTERVAL) {
  // 开始眨眼
  vrmModel.expressionManager.setValue('blink', 1.0);
  lastBlinkTime = currentTime;
} else if (timeSinceLastBlink > 0.15 && timeSinceLastBlink < 0.25) {
  // 0.15秒后睁开
  vrmModel.expressionManager.setValue('blink', 0.0);
}
```

---

## 🎯 完整代码示例

### 示例1：基础骨骼动画
```typescript
// 加载模型后
const vrmModel = model.userData.vrm;
vrmModel.scene.position.set(0, -0.3, 0);
vrmModel.scene.rotation.y = Math.PI;
scene.add(vrmModel.scene);

// 设置初始姿势
const setInitialPose = () => {
  const leftUpperArm = vrmModel.humanoid.getRawBoneNode('leftUpperArm');
  const rightUpperArm = vrmModel.humanoid.getRawBoneNode('rightUpperArm');
  const leftLowerArm = vrmModel.humanoid.getRawBoneNode('leftLowerArm');
  const rightLowerArm = vrmModel.humanoid.getRawBoneNode('rightLowerArm');

  if (leftUpperArm) {
    leftUpperArm.rotation.x = 0.6;
    leftUpperArm.rotation.z = 0.6;
    leftUpperArm.rotation.y = 0.1;
  }
  if (rightUpperArm) {
    rightUpperArm.rotation.x = 0.6;
    rightUpperArm.rotation.z = -0.6;
    rightUpperArm.rotation.y = -0.1;
  }
  if (leftLowerArm) {
    leftLowerArm.rotation.x = 0.4;
  }
  if (rightLowerArm) {
    rightLowerArm.rotation.x = 0.4;
  }
};
setInitialPose();

// 动画循环
let animationTime = 0;
let lastBlinkTime = 0;
const BLINK_INTERVAL = 5;

const updateAnimations = () => {
  animationTime += 0.016;
  const currentTime = Date.now() / 1000;
  
  // 头部微动
  const head = vrmModel.humanoid.getRawBoneNode('head');
  if (head) {
    head.rotation.y = Math.sin(animationTime * 0.8) * 0.15;
    head.rotation.x = Math.sin(animationTime * 0.5) * 0.05;
  }
  
  // 呼吸感
  const spine = vrmModel.humanoid.getRawBoneNode('spine');
  if (spine) {
    spine.position.y = Math.sin(animationTime * 0.6) * 0.01;
  }
  
  // 眨眼
  if (vrmModel.expressionManager) {
    const timeSinceLastBlink = currentTime - lastBlinkTime;
    if (timeSinceLastBlink > BLINK_INTERVAL) {
      vrmModel.expressionManager.setValue('blink', 1.0);
      lastBlinkTime = currentTime;
    } else if (timeSinceLastBlink > 0.15 && timeSinceLastBlink < 0.25) {
      vrmModel.expressionManager.setValue('blink', 0.0);
    }
  }
};
```

---

## 📖 参考资料

### 官方文档
- 主页：https://pixiv.github.io/three-vrm/docs/
- API 参考：https://pixiv.github.io/three-vrm/docs/modules/three-vrm.html
- GitHub：https://github.com/pixiv/three-vrm

### 学习资源
- 官方示例：https://github.com/pixiv/three-vrm/tree/dev/packages/three-vrm/examples
  - 重点查看 `expressions.html`

---

## 🚀 开发进度

### Phase 1：基础功能
- ✅ 模型加载
- ✅ 相机设置
- ✅ 基础位置调整

### Phase 2：基础动作
- ✅ 手臂放下姿势
- ✅ 简单的头部微动
- ✅ 眨眼动画

### Phase 3：表情动作
- [ ] 根据情绪显示表情（高兴/生气/悲伤/思考）
- [ ] 口型同步（待XTTS集成）

### Phase 4：完整功能
- [ ] 语音合成
- [ ] 口型同步
- [ ] 完整的情绪动作系统

---

## ⚠️ 注意事项

1. **骨骼名称是小驼峰！**
   - 错误：`LeftUpperArm`
   - 正确：`leftUpperArm`

2. **表情名称大多数是小驼峰，但自定义表情可能不是！**
   - 例如：`Surprised` 是大写开头

3. **getBoneNode 已弃用！**
   - 请使用：`getRawBoneNode()` 或 `getNormalizedBoneNode()`

4. **Canvas要在DOM中存在！**
   - 不要用v-if来条件渲染Canvas
   - 始终渲染Canvas，用CSS控制显示隐藏

5. **expressionManager.update() 很重要！**
   - 每帧动画后必须调用：`vrmModel.expressionManager.update()`
   - **不要**调用：`vrmModel.update()`（这会导致模型变色！

6. **VRM模型路径！**
   - VRM模型必须放在 `public/` 目录下，不能放在 `assets/` 目录
   - 正确路径：`/3d-models/your-model.vrm`

7. **模型加载失败处理！**
   - 实现重试机制（建议重试3次）
   - 显示错误提示和重试按钮

8. **vrmModel.scene 可能为空！**
   - 加载后先检查：`if (vrmModel && vrmModel.scene)`
