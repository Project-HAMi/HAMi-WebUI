<template>
  <div class="reset-password">
    <el-form
      ref="ruleFormRef"
      style="width: 600px"
      :model="ruleForm"
      status-icon
      :rules="rules"
      label-width="auto"
      class="reset-ruleForm"
      size="large"
    >
      <h2 class="login-title">密码重置</h2>
      <el-form-item label="用户名" prop="userName" class="login-input">
        <el-input
          style="width: 60%"
          class="input"
          v-model="ruleForm.userName"
          placeholder="请输入账号"
        >
        </el-input>
        <el-button
          :loading="codeLoading"
          type="primary"
          @click="getCode()"
          class="code-btn"
        >
          {{ codeLoading ? `${countdown} 秒后重新发送` : '发送验证码' }}
        </el-button>
      </el-form-item>
      <el-form-item label="验证码" prop="code" class="login-input">
        <el-input
          class="input"
          v-model="ruleForm.code"
          placeholder="请输入验证码"
        >
        </el-input>
      </el-form-item>
      <el-form-item label="新密码" prop="pass">
        <el-input
          v-model="ruleForm.pass"
          type="password"
          autocomplete="off"
          placeholder="请输入新密码"
        />
      </el-form-item>
      <el-form-item label="确认密码" prop="newPassword">
        <el-input
          v-model="ruleForm.newPassword"
          type="password"
          autocomplete="off"
          placeholder="请再次输入新密码"
        />
      </el-form-item>
      <el-form-item>
        <el-button
          class="btn-submit"
          type="primary"
          @click="submitForm(ruleFormRef)"
        >
          确认
        </el-button>
      </el-form-item>
     <el-form-item>
        <h2 style="position: absolute; right: 0; top: 5px; cursor: pointer;" @click="()=>{router.push('/admin/login')}"><el-icon size="36px" color="#eee"><Back /></el-icon><span style="position: relative;top: -12px;">返回登录页</span></h2>
      </el-form-item>
    </el-form>
  </div>
  <Bg />
</template>

<script setup>
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Back } from '@element-plus/icons-vue';
import { ElMessage, ElLoading } from 'element-plus';
import { sendCode, resetPassword } from '@/api/user';
import Bg from '@/components/Bg';
const router = useRouter();
const ruleFormRef = ref();
const codeLoading = ref(false);
const countdown = ref(10);
const validatePass = (rule, value, callback) => {
  if (value === '') {
    callback(new Error('请输入密码'));
  } else {
    if (ruleForm.newPassword !== '') {
      if (!ruleFormRef.value) return;
      ruleFormRef.value.validateField('newPassword');
    }
    callback();
  }
};
const validatePass2 = (rule, value, callback) => {
  if (value === '') {
    callback(new Error('请重新输入密码'));
  } else if (value !== ruleForm.pass) {
    callback(new Error('密码不一致!'));
  } else {
    callback();
  }
};

const ruleForm = reactive({
  userName: '',
  code: '',
  pass: '',
  newPassword: '',
});

const rules = reactive({
  pass: [{ required: true, validator: validatePass, trigger: 'blur' }],
  newPassword: [{ required: true, validator: validatePass2, trigger: 'blur' }],
  userName: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  code: [{ required: true, message: '请输入验证码', trigger: 'blur' }],
});

const submitForm = async (ruleFormRef) => {
  if (!ruleFormRef) return;
  await ruleFormRef.validate((valid, fields) => {
    if (valid) {
      const loading = ElLoading.service({
        lock: true,
        text: 'Loading',
        background: 'rgba(0, 0, 0, 0.7)',
      });
      const { userName, code, newPassword } = ruleForm;
      resetPassword({ username: userName, code, newPassword })
        .then((res) => {
          if (res.code === 0) {
            loading.close();
            ElMessage({
              message: '密码重置成功.',
              type: 'success',
            });
            router.push({ path: '/admin/login' });
          }
        })
        .catch((err) => {
          loading.close();
          ElMessage({
            message: '密码重置失败.',
            type: 'warning',
          });
        });
    }
  });
};

const getCode = () => {
  if (ruleForm.userName === '') {
    ElMessage({
      message: '请输入账号.',
      type: 'warning',
    });
  } else {
    sendCode({ username: ruleForm.userName, scene: 'ResetPwd' })
      .then((res) => {
        if (res.code === 0) {
          if (!codeLoading.value) {
            // 发送验证码的逻辑
            codeLoading.value = true;
            const timer = setInterval(() => {
              if (countdown.value > 0) {
                countdown.value--;
              } else {
                clearInterval(timer);
                codeLoading.value = false;
                countdown.value = 10;
              }
            }, 1000);
          }
          ElMessage({
            message: '验证码已发送，请去邮箱查收.',
            type: 'success',
          });
        }
      })
      .catch((err) => {
        ElMessage({
          message: '验证码发送异常，请重试.',
          type: 'warning',
        });
      });
  }
};
</script>

<style lang="scss" scoped>
.reset-password {
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
}
.el-form-item {
  border-radius: 5px;
  color: #eee;
  margin-bottom: 28px;
}
.reset-ruleForm {
  background: rgba(10, 10, 10, 0.17);
  border-radius: 50px;
  padding: 40px;
  .login-title {
    font-size: 26px;
    color: #eee;
    margin: 0px auto 40px auto;
    text-align: center;
    font-weight: bold;
  }
  .btn-submit {
    width: 100%;
    line-height: 44px;
    height: 44px;
    border: none;
    background-color: #1fd1f9;
    background-image: linear-gradient(315deg, #1fd1f9 0, #b621fe 74%);
    &:hover {
    background-color: #1fd1f9;
    background-image: linear-gradient(315deg, #1fd1f9 0, #b621fe 0%);
    transition: all .3s ease;
    }
  }
  .code-btn {
    width: 35%;
    margin-left: 5%;
    line-height: 48px;
    height: 48px;
    border: none;
    background-color: rgb(182 33 254 / 60%);
  }
  .input-icon {
    cursor: pointer;
    width: 25px;
    height: 15px;
    padding-right: 6px;
    padding-bottom: 10px;
  }
}
::v-deep .el-input__wrapper {
  background: rgba(0, 0, 0, 0.1);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.1) !important;
}
::v-deep .el-input__inner,
::v-deep .el-form-item__label {
  color: #fff;
  line-height: 48px;
  height: 48px;
  font-size: 16px;
}
.el-button--primary {
  color: #ffffff;
  background-color: #1890ff;
  border-color: #1890ff;
}
::v-deep .el-input__suffix {
  font-size: 18px;
}
::v-deep .el-input__prefix {
  font-size: 20px;
}
::v-deep .el-form-item__label {
  color: #eee;
}
::v-deep input:-webkit-autofill, 
::v-deep input:-webkit-autofill:hover, 
::v-deep input:-webkit-autofill:focus, 
::v-deep input:-webkit-autofill:active {
    -webkit-box-shadow: 0 0 0px 1000px none inset;
    border: none;
    -webkit-text-fill-color: #fff;
    transition: background-color 5000s ease-in-out 0s;
}
</style>
@/api2/user