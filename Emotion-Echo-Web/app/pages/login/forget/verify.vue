<template>
  <!-- 加动态类区分移动端 -->
  <div class="verify-container" :class="{ 'mobile-verify-container': $device.isMobile }">
    <h2 class="title">确认账号</h2>
    <el-form ref="formRef" :model="formInfo" :rules="rules" class="form">
      <el-form-item prop="username" label="账号" label-position="top">
        <el-input
          v-model="formInfo.username"
          placeholder="请输入账号"
          class="input"
        />
      </el-form-item>

      <el-form-item prop="verificationCode" label="验证码" label-position="top">
        <el-input
          v-model="formInfo.verificationCode"
          placeholder="请输入验证码"
          class="input"
        >
          <template #append>
            <el-button
              @click="getVerificationCode"
              :disabled="isGetVerificationCode"
              type="primary"
              :size="$device.isMobile ? 'small' : 'default'"
            >
              {{
                isGetVerificationCode
                  ? lastSeconds + verificationCodeText
                  : "获取验证码"
              }}
            </el-button>
          </template>
        </el-input>
      </el-form-item>
    </el-form>
    <el-button class="btn" type="primary" @click="gotoModify">
      确认账号
    </el-button>
  </div>
</template>

<script setup lang="ts">
import { ElNotification } from "element-plus";
import { useForgetPwdState } from "~/composables/forgetPwdState";
import { verificationCodeCountDown } from "~/composables/verificationCodeCountDown";
import { phoneOrEmailReg } from "~/utils/Regs";

const emits = defineEmits(["changeActive"]);
const formRef = ref();
// 表单数据
const formInfo = ref({
  username: "",
  verificationCode: "",
});
const rules = ref({
  username: [
    { required: true, message: "请输入手机号或邮箱", trigger: "blur" },
    {
      pattern: phoneOrEmailReg,
      message: "请输入有效的手机号或邮箱",
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

const { startCountdown, isGetVerificationCode, lastSeconds } =
  verificationCodeCountDown();

const userStore = useUserStore();
const { updateStep, userAccount, verificationCode } = useForgetPwdState();

const verificationCodeText = "秒后重新获取";
const getVerificationCode = () => {
  formRef.value?.validateField("username", async (isValid: boolean) => {
    if (!isValid) {
      ElNotification({
        title: "无法获取验证码",
        message: "请填写正确的账户",
        type: "error",
      });
      return;
    }
    const res = await userStore.sendVerificationCode({
      username: formInfo.value.username,
      type: "reset",
    });
    if (!res.isOk) {
      ElNotification({
        title: "验证码发送失败",
        message: res.msg,
        type: "error",
      });
      return;
    }
    ElNotification({
      title: "验证码已发送",
      message: "请注意查收",
      type: "success",
    });
    startCountdown();
  });
};
const gotoModify = () => {
  formRef.value.validate((valid: boolean) => {
    if (!valid) return;
    // 前端校验验证码格式
    if (!/^\d{6}$/.test(formInfo.value.verificationCode)) {
      ElNotification({
        title: "验证码格式错误",
        message: "验证码为6位数字",
        type: "error",
      });
      return;
    }
    // 保存账号和验证码到全局状态
    userAccount.value = formInfo.value.username;
    verificationCode.value = formInfo.value.verificationCode;
    navigateTo("/login/forget/modify");
    updateStep(1);
    emits("changeActive");
  });
};
</script>

<style scoped lang="scss">
// PC端样式（原有样式保留）
.verify-container {
  box-shadow: $box-shadow;
  margin: 100px auto;
  padding: 30px 40px;
  background-color: #fff;
  width: 30vw;
  border-radius: $radius-lg;

  .title {
    margin-bottom: 10px;
    text-align: center;
    font-size: 20px;
  }
  .form {
    .input {
      height: 40px;
      width: 100%;
      margin-bottom: 15px;
      border-radius: $radius-mid;
    }
  }
  .btn {
    height: 40px;
    margin: 10px 0;
    width: 100%;
    border-radius: $radius-mid;
  }
}

// 移动端适配样式
.mobile-verify-container {
  width: 100% !important; // 移动端占90%宽度
  margin: 50px auto !important; // 减少上下间距
  padding: 20px 15px !important; // 减少内边距

  .title {
    font-size: 18px !important; // 缩小标题字体
    margin-bottom: 15px !important;
  }
  .form {
    .input {
      height: 44px !important; // 移动端输入框高度适配触摸
      margin-bottom: 20px !important;
    }

  }
  .btn {
    height: 44px !important; // 移动端按钮高度适配触摸
    font-size: 16px !important;
    margin: 15px 0 !important;
  }
}

// 平板端兜底
@media (max-width: 1024px) and (min-width: 768px) {
  .verify-container {
    width: 50vw !important;
    padding: 25px 30px !important;
  }
}
</style>