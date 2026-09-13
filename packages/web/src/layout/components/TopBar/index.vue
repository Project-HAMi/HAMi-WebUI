<template>
  <div id="topBar">
    <div class="left">
      <div class="logo">
        <Logo />
      </div>
    </div>
    <div class="right">
      <LangSelect />
    </div>
  </div>
</template>

<script setup>
import Logo from './logo.vue';
import LangSelect from '@/components/LangSelect/index.vue';

import './style.scss';
</script>

<style lang="scss"></style>
