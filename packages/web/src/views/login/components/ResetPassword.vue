<template>
  <div>
    <el-dialog v-model="resetDialog" title="密码重置" width="500">
      <el-form
        ref="resetFormRef"
        style="max-width: 600px"
        :model="resetForm"
        status-icon
        :rules="rules"
        label-width="auto"
      >
        <el-form-item label="用户名" prop="userName" class="login-input">
          <el-input
            style="width: 70%"
            class="input"
            v-model="resetForm.userName"
            placeholder="请输入账号"
          >
          </el-input>
          <el-button
            type="primary"
            @click="getcode()"
            style="width: 20%; margin-left: 10%"
          >
            发送验证码
          </el-button>
        </el-form-item>
        <el-form-item label="验证码" prop="code" class="login-input">
          <el-input
            class="input"
            v-model="resetForm.code"
            placeholder="请输入账号"
          >
          </el-input>
        </el-form-item>
        <el-form-item label="新密码" prop="pass">
          <el-input
            v-model="resetForm.pass"
            type="password"
            autocomplete="off"
          />
        </el-form-item>
        <el-form-item label="确认密码" prop="newPassword">
          <el-input
            v-model="resetForm.newPassword"
            type="password"
            autocomplete="off"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button type="primary" @click="resetPas(resetFormRef)">
            确认
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { reactive, ref, inject } from 'vue';
import { ElMessage } from 'element-plus';
import { sendCode, resetPassword } from '@/api/user';
const resetFormRef = ref();
const resetDialog = inject('resetDialog');
const validatePass = (rule, value, callback) => {
  if (value === '') {
    callback(new Error('请输入密码'));
  } else {
    if (resetForm.newPassword !== '') {
      if (!resetFormRef.value) return;
      resetFormRef.value.validateField('newPassword');
    }
    callback();
  }
};
const validatePass2 = (rule, value, callback) => {
  if (value === '') {
    callback(new Error('请重新输入密码'));
  } else if (value !== resetForm.pass) {
    callback(new Error('密码不一致!'));
  } else {
    callback();
  }
};

const resetForm = reactive({
  userName: '',
  code: '',
  pass: '',
  newPassword: '',
});

const rules = reactive({
  pass: [{ validator: validatePass, trigger: 'blur' }],
  newPassword: [{ validator: validatePass2, trigger: 'blur' }],
  userName: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  code: [{ required: true, message: '请输入验证码', trigger: 'blur' }],
});

const resetPas = async (resetFormRef) => {
  if (!resetFormRef) return;
  await resetFormRef.validate((valid, fields) => {
    if (valid) {
      const { userName, code, newPassword } = resetForm;
      resetPassword({ username: userName, code, newPassword })
        .then((res) => {
          if (res.code === 0) {
            ElMessage({
              message: '密码重置成功.',
              type: 'success',
            });
          }
        })
        .catch((err) => {
          ElMessage({
            message: '密码重置失败.',
            type: 'warning',
          });
        });
    }
  });
};

const getcode = () => {
  if (resetForm.userName === '') {
    ElMessage({
      message: '请输入账号.',
      type: 'warning',
    });
  } else {
    sendCode({ username: resetForm.userName, scene: 'ResetPwd' })
      .then((res) => {
        if (res.code === 0) {
          ElMessage({
            message: '验证码已发送，请去邮箱查收.',
            type: 'success',
          });
        }
      })
      .catch((err) => {
        ElMessage({
          message: '验证码已发送异常，请重试.',
          type: 'warning',
        });
      });
  }
};
</script>
@/api2/user