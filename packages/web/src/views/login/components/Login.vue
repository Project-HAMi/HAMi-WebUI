<template>
  <el-form
    ref="ruleFormRef"
    style="max-width: 600px; min-width: 550px"
    :model="ruleForm"
    :rules="rules"
    label-width="auto"
    class="login-ruleForm"
  >
    <el-form-item><h2 class="login-title">账号登录</h2></el-form-item>
    <el-form-item label="" prop="userName" class="login-input">
      <el-input
        class="input"
        :prefix-icon="User"
        v-model="ruleForm.userName"
        placeholder="请输入账号"
        @keyup.enter="login(ruleFormRef)"
      >
      </el-input>
    </el-form-item>
    <el-form-item label="" prop="password" class="login-input">
      <el-input
        v-model="ruleForm.password"
        :type="addPassFlag ? 'text' : 'password'"
        placeholder="请输入密码"
        :prefix-icon="Lock"
        @keyup.enter="login(ruleFormRef)"
      >
        <template #suffix>
          <span @click="addPassFlag = !addPassFlag" style="cursor: pointer;">
            <el-icon v-if="addPassFlag"><View /></el-icon>
            <el-icon v-else><Hide /></el-icon>
          </span>
        </template>
      </el-input>
    </el-form-item>
    <el-form-item>
      <span class="reset" @click="resetForm()">忘记密码？</span>
    </el-form-item>
    <el-form-item>
      <el-button
        class="btn-submit"
        type="primary"
        @click="login(ruleFormRef)"
      >
        登录
      </el-button>
    </el-form-item>
  </el-form>
</template>

<script setup>
import { reactive, ref, watch } from 'vue';
import { useStore } from 'vuex'
import { useRouter, useRoute } from 'vue-router';
import { User, Lock, View, Hide } from '@element-plus/icons-vue';
import { ElLoading } from 'element-plus';
const store = useStore()
const router = useRouter();
const route = useRoute();
const ruleFormRef = ref();
const addPassFlag = ref(false); //图标显示标识
const redirect = ref(undefined);
const otherQuery = ref({});
const ruleForm = reactive({
  userName: '',
  password: '',
});
const rules = reactive({
  userName: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
});
const getOtherQuery = (query) => {
      return Object.keys(query).reduce((acc, cur) => {
        if (cur !== 'redirect') {
          acc[cur] = query[cur]
        }
        return acc
      }, {})
    }
watch(
  () => route,
  (route) => {
    const query = route.query;
    if (query) {
      redirect.value = query.redirect;
      otherQuery.value = getOtherQuery(query);
    }
  },
  { immediate: true }
);
const login = async (ruleFormRef) => {
  if (!ruleFormRef) return;
  await ruleFormRef.validate((valid, fields) => {
    if (valid) {
      const loading = ElLoading.service({
        lock: true,
        text: 'Loading',
        background: 'rgba(0, 0, 0, 0.7)',
      });
      const { userName, password } = ruleForm;
      store.dispatch('user/login',{username: userName,passwordPlaintext: password,})
      .then(res=>{
        router.push({ path: redirect.value || '/', query: otherQuery.value })
        loading.close()
      }).catch(err=>{
        loading.close()
      });
    }
  });
};
const resetForm = () => {
  router.push('/admin/reset-password');
};
</script>

<style lang="scss" scoped>
.login-ruleForm {
  background: rgba(10, 10, 10, 0.17);
  padding: 40px;
  border-bottom: 8px solid #b621fe;
  border-radius: 30px 30px 50px 50px;
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
  .reset {
    display: inline-block;
    font-size: 16px;
    color: #eee;
    cursor: pointer;
    position: absolute;
    right: 0px;
    top: 0px;
  }
  .other-submit {
    width: 30%;
  }
  .input-icon {
    cursor: pointer;
    width: 25px;
    height: 15px;
    padding-right: 6px;
    padding-bottom: 10px;
  }
}
.other-submit {
  width: 30%;
}
.input-icon {
  cursor: pointer;
  width: 25px;
  height: 15px;
  padding-right: 6px;
  padding-bottom: 10px;
}
::v-deep .el-checkbox__input.is-checked .el-checkbox__inner {
    background-color: #b621fe;
    border-color: #b621fe;
}
::v-deep .el-input__wrapper {
  background: rgba(0, 0, 0, 0.1);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.1) !important;
}
::v-deep .el-input__inner {
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
::v-deep .el-checkbox__label {
  color: #eee !important;
  font-size: 16px !important;
}
.el-form-item {
  margin-bottom: 28px;
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
