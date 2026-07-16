<template>
  <!-- 外层容器：根据设备切换布局逻辑 -->
  <div
    class="login-container"
    :class="{ 'mobile-container': $device.isMobile }"
  >
    <!-- PC端mask（移动端隐藏） -->
    <div class="mask pc-mask" ref="mask" v-if="!$device.isMobile">
      <h2 class="mask-title">
        <template v-for="(line, index) in splitPoem" :key="index">
          <p class="poem-line">{{ line }}</p>
        </template>
      </h2>
      <p class="mask-desc">
        {{ isLogin ? "还没有账号？去注册" : "已经有账号了？去登陆" }}
      </p>
      <el-button
        class="mask-btn"
        :circle="true"
        @click="switchMask"
        :disabled="isDisable"
      >
        <el-icon size="30px"><Switch /></el-icon>
      </el-button>
    </div>

    <!-- 移动端顶部提示区（PC端隐藏） -->
    <div class="mobile-mask" v-if="$device.isMobile">
      <h2 class="mobile-mask-title">
        <template v-for="(line, index) in splitPoem" :key="index">
          <p class="mobile-poem-line">{{ line }}</p>
        </template>
      </h2>
      <p class="mobile-mask-desc">
        {{
          isLogin
            ? "还没有账号？点击下方按钮注册"
            : "已经有账号了？点击下方按钮登陆"
        }}
      </p>
      <el-button
        class="mobile-mask-btn"
        type="primary"
        @click="switchMobileForm"
        size="small"
      >
        {{ isLogin ? "去注册" : "去登陆" }}
      </el-button>
    </div>

    <!-- PC端注册表单（绝对定位） -->
    <el-form
      class="register pc-form"
      ref="registerFormRef"
      :model="registerInfo"
      :rules="registerRules"
      v-if="!$device.isMobile"
    >
      <h1 class="register-title">注册</h1>
      <el-form-item label="账号" label-position="top" prop="username">
        <el-input
          v-model="registerInfo.username"
          class="register-input"
          placeholder="请输入手机号/邮箱"
        />
      </el-form-item>
      <el-form-item label="密码" label-position="top" prop="password">
        <el-input
          v-model="registerInfo.password"
          type="password"
          :show-password="true"
          class="register-input"
          placeholder="请输入密码"
        />
      </el-form-item>
      <el-form-item label="验证码" label-position="top" prop="verificationCode">
        <el-input
          v-model="registerInfo.verificationCode"
          class="register-input"
          placeholder="请输入验证码"
        >
          <template #append>
            <el-button
              @click="getVerificationCode"
              ref="verificationCodeRef"
              :disabled="isGetVerificationCode"
              type="primary"
              >{{
                isGetVerificationCode
                  ? lastSeconds + verificationCodeText
                  : "获取验证码"
              }}</el-button
            >
          </template>
        </el-input>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" class="register-btn" @click="registerHandler">
          注册
        </el-button>
      </el-form-item>
    </el-form>

    <!-- PC端登录表单（绝对定位） -->
    <el-form
      class="login pc-form"
      ref="loginFormRef"
      :model="loginInfo"
      :rules="loginRules"
      v-if="!$device.isMobile"
    >
      <h1 class="login-title">登陆</h1>
      <el-form-item label="账号" label-position="top" prop="username">
        <el-input
          v-model="loginInfo.username"
          class="login-input"
          placeholder="请输入手机号/邮箱"
        />
      </el-form-item>
      <el-form-item label="密码" label-position="top" prop="password">
        <el-input
          v-model="loginInfo.password"
          type="password"
          :show-password="true"
          class="login-input"
          placeholder="请输入密码"
        />
      </el-form-item>
      <div class="login-text">
        <el-checkbox label="记住我" v-model="isRemember" />
        <NuxtLink to="/login/forget/verify">忘记密码</NuxtLink>
      </div>
      <el-form-item>
        <el-button type="primary" class="login-btn" @click="loginHandler">
          登陆
        </el-button>
      </el-form-item>
      <div class="other-way">
        <img src="/assets/icons/微信.svg" alt="" @click="wechatLogin" />
        <img src="/assets/icons/腾讯QQ.svg" alt="" @click="QQLogin" />
      </div>
    </el-form>

    <!-- 移动端表单区（PC端隐藏） -->
    <div class="mobile-form-wrap" v-if="$device.isMobile">
      <!-- 移动端注册表单 -->
      <el-form
        class="register mobile-form"
        ref="registerFormRef"
        :model="registerInfo"
        :rules="registerRules"
        v-show="!isLogin"
      >
        <h1 class="register-title">注册</h1>
        <el-form-item label="账号" label-position="top" prop="username">
          <el-input
            v-model="registerInfo.username"
            class="register-input"
            placeholder="请输入手机号/邮箱"
          />
        </el-form-item>
        <el-form-item label="密码" label-position="top" prop="password">
          <el-input
            v-model="registerInfo.password"
            type="password"
            :show-password="true"
            class="register-input"
            placeholder="请输入密码"
          />
        </el-form-item>
        <el-form-item label="验证码" label-position="top" prop="verificationCode">
          <el-input
            v-model="registerInfo.verificationCode"
            class="register-input"
            placeholder="请输入验证码"
          >
            <template #append>
              <el-button
                @click="getVerificationCode"
                ref="verificationCodeRef"
                :disabled="isGetVerificationCode"
                type="primary"
                size="small"
                >{{
                  isGetVerificationCode
                    ? lastSeconds + verificationCodeText
                    : "获取验证码"
                }}</el-button
              >
            </template>
          </el-input>
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            class="register-btn"
            @click="registerHandler"
          >
            注册
          </el-button>
        </el-form-item>
      </el-form>

      <!-- 移动端登录表单 -->
      <el-form
        class="login mobile-form"
        ref="loginFormRef"
        :model="loginInfo"
        :rules="loginRules"
        v-show="isLogin"
      >
        <h1 class="login-title">登陆</h1>
        <el-form-item label="账号" label-position="top" prop="username">
          <el-input
            v-model="loginInfo.username"
            class="login-input"
            placeholder="请输入手机号/邮箱"
          />
        </el-form-item>
        <el-form-item label="密码" label-position="top" prop="password">
          <el-input
            v-model="loginInfo.password"
            type="password"
            :show-password="true"
            class="login-input"
            placeholder="请输入密码"
          />
        </el-form-item>
        <div class="login-text">
          <el-checkbox label="记住我" v-model="isRemember" size="small" />
          <NuxtLink to="/login/forget/verify">忘记密码</NuxtLink>
        </div>
        <el-form-item>
          <el-button type="primary" class="login-btn" @click="loginHandler">
            登陆
          </el-button>
        </el-form-item>
        <div class="other-way">
          <img src="/assets/icons/微信.svg" alt="" @click="wechatLogin" />
          <img src="/assets/icons/腾讯QQ.svg" alt="" @click="QQLogin" />
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ElNotification } from "element-plus";
import { Switch } from "@element-plus/icons-vue";
import type { loginInfo, registerInfo } from "~/types/login/loginType";
import { sha256 } from "js-sha256";
import { verificationCodeCountDown } from "~/composables/verificationCodeCountDown";
const { isMobile } = useDevice();
// 正则定义（补充缺失的正则）

const originalWarn = console.warn;
const originalLog = console.log;
console.warn = function (...args) {
  if (args.some((arg) => arg?.includes?.("async-validator"))) return;
  originalWarn.apply(console, args);
};
console.log = function (...args) {
  if (args.some((arg) => arg?.includes?.("async-validator"))) return;
  originalLog.apply(console, args);
};

onUnmounted(() => {
  console.warn = originalWarn;
  console.log = originalLog;
});

const loginFormRef = ref();
const registerFormRef = ref();
const verificationCodeRef = ref();
const loginInfo = shallowReactive<loginInfo>({
  username: "",
  password: "",
});
const registerInfo = shallowReactive<registerInfo>({
  username: "",
  password: "",
  verificationCode: "",
});

const loginRules = ref({
  username: [
    { required: true, message: "请输入手机号/邮箱", trigger: "blur" },
    {
      pattern: phoneOrEmailReg,
      message: "请输入有效的手机号或邮箱",
      trigger: "blur",
    },
  ],
  password: [
    { required: true, message: "请输入密码", trigger: "blur" },
    {
      pattern: passwordReg,
      message: "密码需为6-18位，且包含字母和数字",
      trigger: "blur",
    },
  ],
});

const registerRules = ref({
  username: [
    { required: true, message: "请输入手机号/邮箱", trigger: "blur" },
    {
      pattern: phoneOrEmailReg,
      message: "请输入有效的手机号或邮箱",
      trigger: "blur",
    },
  ],
  password: [
    { required: true, message: "请输入密码", trigger: "blur" },
    {
      pattern: passwordReg,
      message: "密码需为6-18位，且包含字母和数字",
      trigger: "blur",
    },
  ],
  verificationCode: [
    { required: true, message: "请输入验证码", trigger: "blur" },
    {
      pattern: /^\d{6}$/,
      message: "验证码为6位数字",
      trigger: "blur",
    },
  ],
});

const isDisable = ref(false);
const isRemember = ref<boolean>(false);

// 页面加载时恢复记住我状态
onMounted(() => {
  if (import.meta.client) {
    const stored = localStorage.getItem("remember_me");
    if (stored !== null) {
      isRemember.value = stored === "true";
    }
  }
});
const isLogin = ref<boolean>(true);
const mask = ref<HTMLDivElement>();
const welcomePoems = [
  "有朋自远方来， 不亦乐乎?",
  "花径不曾缘客扫， 蓬门今始为君开。",
  "晚来天欲雪， 能饮一杯无？",
  "开轩面场圃， 把酒话桑麻。",
  "正是江南好风景， 落花时节又逢君。",
];

const poem = useState("random-poem", () => {
  const randomIndex = Math.floor(Math.random() * welcomePoems.length);
  return welcomePoems[randomIndex] as string;
});

const splitPoem = computed(() => {
  return poem.value
    .split(" ")
    .map((line) => line.trim())
    .filter((line) => line);
});

// PC端mask切换逻辑
const switchMask = () => {
  isLogin.value = !isLogin.value;
  if (mask.value && !isMobile) {
    mask.value.classList.remove("mask-register", "mask-login");
    mask.value?.classList.add(!isLogin.value ? "mask-register" : "mask-login");
  }
  isDisable.value = true;
  let timer = setTimeout(() => {
    isDisable.value = false;
    clearTimeout(timer);
  }, 2000);
};

// 移动端表单切换逻辑（简化，无动画）
const switchMobileForm = () => {
  isLogin.value = !isLogin.value;
};

const loginHandler = () => {
  loginFormRef.value.validate((valid: boolean) => {
    if (valid) {
      // 校验通过，执行登录逻辑（trim 去除前后空格）
      handleLogin(loginInfo.username.trim(), loginInfo.password.trim());
    }
  });
};

/**
 * 处理账号密码登录
 * @param username 用户名（手机号/邮箱）
 * @param password 密码（已哈希）
 */
const userStore = useUserStore();

const handleLogin = async (username: string, password: string) => {
  // 持久化记住我状态
  if (import.meta.client) {
    localStorage.setItem("remember_me", String(isRemember.value));
  }

  const result = await userStore.login({
    username: username.trim(),
    password: sha256(password.trim()),
    rememberMe: isRemember.value,
  });

  if (result.isOk) {
    ElNotification({
      title: "登录成功",
      type: "success",
    });
    navigateTo("/chat/conversation");
  } else {
    ElNotification({
      title: "登录失败",
      message: result.msg,
      type: "error",
    });
  }
};
const registerHandler = () => {
  registerFormRef.value.validate((valid: boolean) => {
    if (valid) {
      // 校验通过，执行注册逻辑（trim 去除前后空格）
      handleRegister(
        registerInfo.username.trim(),
        registerInfo.password.trim(),
        registerInfo.verificationCode.trim()
      );
    }
  });
};

/**
 * 处理用户注册
 * @param username 用户名（手机号/邮箱）
 * @param password 密码
 * @param verificationCode 验证码
 */
const handleRegister = async (username: string, password: string, verificationCode: string) => {
  const result = await userStore.register({
    username: username.trim(),
    password: sha256(password.trim()),
    verificationCode: verificationCode.trim(),
  });
  
  if (result.isOk) {
    ElNotification({
      title: "注册成功",
      message: "已自动登录",
      type: "success",
    });
    navigateTo("/chat/conversation");
  } else {
    ElNotification({
      title: "注册失败",
      message: result.msg,
      type: "error",
    });
  }
};

const { startCountdown, isGetVerificationCode, lastSeconds, verificationCodeText } =
  verificationCodeCountDown();
const getVerificationCode = async () => {
  registerInfo.username = registerInfo.username.trim();
  if (!phoneOrEmailReg.test(registerInfo.username)) {
    ElNotification({
      title: "无法获取验证码",
      message: "请填写正确的账户",
      type: "error",
    });
    return;
  }
  
  const result = await userStore.sendVerificationCode({
    username: registerInfo.username,
    type: "register",
  });
  
  if (result.isOk) {
    ElNotification({
      title: "验证码已发送",
      message: "请注意查收",
      type: "success",
    });
    startCountdown();
  } else {
    ElNotification({
      title: "发送失败",
      message: result.msg,
      type: "error",
    });
  }
};
const wechatLogin = () => {
  // TODO: 微信登录逻辑
  handleOAuthLogin("wechat");
};
const QQLogin = () => {
  // TODO: QQ登录逻辑
  handleOAuthLogin("qq");
};

/**
 * 处理第三方OAuth登录
 * @param provider 登录提供商 wechat | qq
 */
const handleOAuthLogin = (provider: "wechat" | "qq") => {
  // TODO: 
  // 1. 跳转到OAuth授权页面 或 打开弹出窗口
  // 2. 等待授权回调
  // 3. 获取code后发送给后端
  // 4. 后端返回token后登录成功
  console.log("OAuth登录:", provider);
};
</script>

<style scoped lang="scss">
// 原有PC端样式（保留，仅加命名空间）
@keyframes maskAnimationRegister {
  0% {
    width: 50%;
    left: 0;
    border-radius: $radius-lg 160px 160px $radius-lg;
  }
  30% {
    width: 100%;
    left: 0;
    border-radius: $radius-lg;
  }
  50% {
    width: 100%;
    left: 0;
    border-radius: $radius-lg;
  }
  80% {
    width: 50%;
    left: 50%;
    border-radius: 160px $radius-lg $radius-lg 160px;
  }
  100% {
    width: 50%;
    left: 50%;
    border-radius: 160px $radius-lg $radius-lg 160px;
  }
}

@keyframes maskAnimationLogin {
  0% {
    width: 50%;
    left: 50%;
    border-radius: 160px $radius-lg $radius-lg 160px;
  }
  30% {
    width: 100%;
    left: 0;
    border-radius: $radius-lg;
  }
  50% {
    width: 100%;
    left: 0;
    border-radius: $radius-lg;
  }
  80% {
    width: 50%;
    left: 0;
    border-radius: $radius-lg 160px 160px $radius-lg;
  }
  100% {
    width: 50%;
    left: 0;
    border-radius: $radius-lg 160px 160px $radius-lg;
  }
}

.login-container {
  display: flex;
  height: 70vh;
  width: 55vw;
  background-color: #ffffff;
  margin: auto;
  transform: translateY(17vh);
  border-radius: $radius-lg;
  box-shadow: $box-shadow;
  position: relative;

  // PC端mask样式
  .pc-mask {
    position: absolute;
    top: 0;
    left: 0;
    border-radius: $radius-lg 160px 160px $radius-lg;
    width: 50%;
    z-index: 5;
    height: 100%;
    background-color: #0077c2;
    padding-top: 25%;
    padding: 20% 10px 0;

    &.mask-register {
      animation: maskAnimationRegister 2s ease-in-out forwards;
    }

    &.mask-login {
      animation: maskAnimationLogin 2s ease-in-out forwards;
    }

    .mask-title {
      font-family: "maskTitle";
      font-size: 2.3em;
      text-align: center;
      line-height: 1.5;
      display: flex;
      flex-direction: column;
      justify-content: center;
      align-items: center;
    }

    .poem-line {
      display: block;
      width: 100%;
    }

    .mask-desc {
      margin-top: 10px;
      text-align: center;
      font-size: 14px;
      color: #313131;
    }

    .mask-btn {
      display: block;
      margin: 40px auto;
      height: 60px;
      width: 60px;
    }
  }

  // PC端表单通用样式
  .pc-form {
    height: 100%;
    padding: 5vh 30px;
    width: 50%;
    position: absolute;
    overflow: hidden;

    &.login {
      left: 50%;
    }

    &.register {
      left: 0;
    }

    .login-title,
    .register-title {
      text-align: center;
      font-size: 5vh;
      margin-bottom: 2vh;
    }

    .login-input,
    .register-input {
      width: 100%;
      height: 40px;
      margin-bottom: 1vh;
    }

    .login-text {
      display: flex;
      justify-content: space-between;
      font-size: 14px;
      color: #666666;
      margin: 1.5vh 0;

      a {
        line-height: 32px;
        cursor: pointer;
      }
    }

    .login-btn,
    .register-btn {
      width: 100%;
      margin-top: 1.5vh;
      height: 2.5em;
      border-radius: $radius-mid;
      font-size: 18px;
    }

    .other-way {
      display: flex;
      justify-content: space-around;
      margin-top: 3vh;
      img {
        height: 50px;
        object-fit: contain;
      }
    }
  }
}

// 移动端布局样式（核心适配）

.mobile-container {
  width: 95% !important;
  height: auto !important;
  max-height: 90vh !important; // 限制最大高度，避免小屏溢出
  position: absolute !important; // 绝对定位是translate居中的基础
  top: 50% !important; // 容器顶部对齐屏幕垂直中点
  left: 0 !important;
  right: 0 !important;
  margin: 0 auto !important; // 水平居中
  transform: translateY(-50%) !important; // 向上偏移自身50%高度，实现垂直居中
  padding: 10px !important;
  flex-direction: column !important;
  overflow-y: auto; // 小屏时内部滚动，不溢出
}

// 移动端mask（顶部提示区）
.mobile-mask {
  width: 100%;
  background-color: #0088dd;
  border-radius: $radius-lg $radius-lg 0 0;
  padding: 20px 15px;
  text-align: center;

  .mobile-mask-title {
    font-family: "maskTitle";
    font-size: 1.5em;
    color: #fff;
    margin-bottom: 10px;
  }

  .mobile-poem-line {
    display: block;
    margin-bottom: 5px;
  }

  .mobile-mask-desc {
    font-size: 14px;
    color: #f5f5f5;
    margin-bottom: 15px;
  }

  .mobile-mask-btn {
    border-radius: $radius-mid;
    padding: 6px 20px;
  }
}

// 移动端表单容器
.mobile-form-wrap {
  width: 100%;
  padding: 20px 15px;
  background-color: #fff;
  border-radius: 0 0 $radius-lg $radius-lg;
}

// 移动端表单样式
.mobile-form {
  width: 100% !important;
  height: auto !important;
  position: static !important;
  padding: 0 !important;
  margin: 0 !important;

  .login-title,
  .register-title {
    font-size: 24px !important;
    margin-bottom: 20px !important;
    color: #333;
  }

  .login-input,
  .register-input {
    height: 44px !important;
    margin-bottom: 15px !important;
    border-radius: $radius-mid;
    border: 1px solid #e5e7eb;
  }

  .login-text {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin: 10px 0 20px !important;
    font-size: 13px !important;

    a {
      color: #409eff;
      text-decoration: none;
    }
  }

  .login-btn,
  .register-btn {
    width: 100%;
    height: 44px !important;
    font-size: 16px !important;
    border-radius: $radius-mid !important;
  }

  .other-way {
    display: flex;
    justify-content: space-around;
    margin-top: 20px !important;
    img {
      height: 40px !important;
    }
  }
}

// 兜底：小屏PC适配
// @media (max-width: 1024px) and (min-width: 768px) {
//   .login-container {
//     width: 80vw !important;
//   }
// }
</style>
