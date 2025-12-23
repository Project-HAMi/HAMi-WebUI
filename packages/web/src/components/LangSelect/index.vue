<template>
  <el-dropdown trigger="click" @command="handleSetLanguage" popper-class="lang-dropdown-popper">
    <div class="lang-select-container">
      <svg-icon icon="language" class-name="lang-icon" />
      <span class="lang-text">{{ language === 'zh' ? $t('common.lang.zh') : $t('common.lang.en') }}</span>
      <el-icon class="el-icon--right"><arrow-down /></el-icon>
    </div>
    <template #dropdown>
      <el-dropdown-menu class="lang-dropdown">
        <el-dropdown-item 
          command="zh" 
          class="lang-item"
          :class="{ 'is-selected': language === 'zh' }"
        >
          <span class="dropdown-item-text">{{ $t('common.lang.zh') }}</span>
          <span v-if="language === 'zh'" class="check-mark">✓</span>
        </el-dropdown-item>
        <el-dropdown-item 
          command="en" 
          class="lang-item"
          :class="{ 'is-selected': language === 'en' }"
        >
          <span class="dropdown-item-text">{{ $t('common.lang.en') }}</span>
          <span v-if="language === 'en'" class="check-mark">✓</span>
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import Cookies from 'js-cookie';
import { LANG_KEY } from '@/locales';
import { ArrowDown } from '@element-plus/icons-vue';

const i18n = useI18n();

const language = computed(() => i18n.locale.value);

const handleSetLanguage = (lang) => {
  i18n.locale.value = lang;
  Cookies.set(LANG_KEY, lang);
};
</script>

<style lang="scss" scoped>
.lang-select-container {
  cursor: pointer;
  display: flex;
  align-items: center;
  height: 32px;
  padding: 0 12px;
  border-radius: 4px;
  color: rgba(255, 255, 255, 0.9);
  transition: all 0.3s ease;
  border: 1px solid transparent;
  margin-right: 16px;

  &:hover {
    background: rgba(255, 255, 255, 0.1);
    border-color: rgba(255, 255, 255, 0.2);
  }

  .lang-icon {
    font-size: 18px;
    margin-right: 8px;
  }
  
  .lang-text {
    font-size: 14px;
    margin-right: 6px;
    user-select: none;
    font-weight: 500;
  }

  .el-icon--right {
    font-size: 12px;
    opacity: 0.7;
  }
}
</style>

<style lang="scss">
// 全局样式，用于定制 Popper 弹窗
.lang-dropdown-popper {
  .el-dropdown-menu {
    padding: 6px;
    border-radius: 8px;
    border: 1px solid #e4e7ed;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  }

  .el-dropdown-menu__item {
    padding: 8px 16px;
    border-radius: 4px;
    margin: 2px 0;
    line-height: 1.5;
    font-weight: 400; // 默认正常粗细
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-width: 120px;
    color: #606266;
    
    &:hover {
      background-color: #f5f7fa;
      color: var(--el-color-primary);
    }
    
    // 选中状态样式
    &.is-selected {
      color: var(--el-color-primary);
      font-weight: 600;
      background-color: var(--el-color-primary-light-9); // 非常淡的背景，或者直接透明
    }

    .dropdown-item-text {
      flex: 1;
    }

    .check-mark {
      font-weight: bold;
      font-size: 14px;
    }
  }
  
  // 隐藏默认的箭头，看起来更干净
  .el-popper__arrow {
    display: none;
  }
}
</style>
