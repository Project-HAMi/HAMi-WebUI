<template>
  <div class="login">
    <Login />
    <Bg/>
  </div>
</template>

<script setup>
import Login from './components/Login.vue';
import Bg from '@/components/Bg'
</script>

<style lang="scss" scoped>
.login {
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
}
.el-form-item {
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(0, 0, 0, 0.1);
    border-radius: 5px;
    color: #454545;
  }
</style>