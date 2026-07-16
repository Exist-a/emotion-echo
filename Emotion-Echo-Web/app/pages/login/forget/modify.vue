<template>
  <!-- 加动态类区分移动端 -->
  <div class="modify-container" :class="{ 'mobile-modify-container': $device.isMobile }">
    <h2 class="title">修改密码</h2>
    <el-form :rules="rules" :model="formInfo" ref="formRef">
      <el-form-item label="新密码" label-position="top" prop="newPassword">
        <el-input
          placeholder="请输入新密码"
          class="input"
          v-model="formInfo.newPassword"
          :show-password="true"
          type="password"
        />
      </el-form-item>
      <el-form-item
        label="确认密码"
        label-position="top"
        prop="confirmNewPassword"
      >
        <el-input
          placeholder="请再次输入密码"
          class="input"
          v-model="formInfo.confirmNewPassword"
          :show-password="true"
          type="password"
        />
      </el-form-item>
    </el-form>
    <el-button class="btn" type="primary" @click="gotoSuccess">
      确认修改
    </el-button>
  </div>
</template>

<script setup lang="ts">
// 补充缺失的正则和组合式函数（保证代码可运行）
const passwordReg = /^(?=.*[a-zA-Z])(?=.*\d).{6,18}$/;
import { ElNotification } from "element-plus";
import { useForgetPwdState } from "~/composables/forgetPwdState";
import { post } from "~/composables/useApi";
import { sha256 } from "js-sha256";

definePageMeta({
  middleware: "forget-pwd",
});
const emits = defineEmits(["changeActive"]);
const formRef = ref();
const { updateStep, userAccount, verificationCode } = useForgetPwdState();

const gotoSuccess = async () => {
  const valid = await formRef.value.validate().catch(() => false);
  if (!valid) {
    ElNotification({
      type: "error",
      title: "密码修改失败",
      message: "请检查格式",
    });
    return;
  }

  if (!userAccount.value) {
    ElNotification({
      type: "error",
      title: "密码修改失败",
      message: "未获取到账号信息，请返回上一步重新验证",
    });
    return;
  }

  if (!verificationCode.value) {
    ElNotification({
      type: "error",
      title: "密码修改失败",
      message: "未获取到验证码，请返回上一步重新获取",
    });
    return;
  }

  try {
    await post("/auth/reset-password", {
      username: userAccount.value,
      verificationCode: verificationCode.value,
      newPassword: sha256(formInfo.value.newPassword),
    });

    ElNotification({
      type: "success",
      title: "密码修改成功",
      message: "请使用新密码登录",
    });

    updateStep(2);
    navigateTo("/login/forget/success");
    emits("changeActive");
  } catch (error: any) {
    ElNotification({
      type: "error",
      title: "密码修改失败",
      message: error.message || "请稍后重试",
    });
  }
};
const formInfo = ref({
  newPassword: "",
  confirmNewPassword: "",
});
const rules = ref({
  newPassword: [
    { required: true, message: "请输入新密码", trigger: "blur" },
    {
      pattern: passwordReg,
      message: "密码需为6-18位，且包含字母和数字",
      trigger: "blur",
    },
  ],
  confirmNewPassword: [
    { required: true, message: "请输入确认密码", trigger: "blur" },
    {
      validator: (rule: any, value: string, callback: Function) => {
        if (value !== formInfo.value.newPassword) {
          callback(new Error("两次输入的密码不一致"));
        } else {
          callback();
        }
      },
      trigger: "blur",
    },
  ],
});
</script>

<style lang="scss" scoped>
// PC端样式（原有样式保留）
.modify-container {
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
  .btn {
    height: 40px;
    margin: 10px 0;
    width: 100%;
    border-radius: $radius-mid;
  }
  .input {
    height: 40px;
    width: 100%;
    margin-bottom: 15px;
    border-radius: $radius-mid;
  }
}

// 移动端适配样式
.mobile-modify-container {
  width: 100% !important; // 移动端占90%宽度
  margin: 50px auto !important; // 减少上下间距
  padding: 20px 15px !important; // 减少内边距
  box-sizing: border-box;

  .title {
    font-size: 18px !important; // 缩小标题字体
    margin-bottom: 15px !important;
  }
  .input {
    height: 44px !important; // 移动端输入框高度适配触摸
    margin-bottom: 20px !important;
  }
  .btn {
    height: 44px !important; // 移动端按钮高度适配触摸
    font-size: 16px !important;
    margin: 15px 0 !important;
  }
}

// 平板端兜底
@media (max-width: 1024px) and (min-width: 768px) {
  .modify-container {
    width: 50vw !important;
    padding: 25px 30px !important;
  }
}
</style>