<template>
  <div v-if="sidebar && !collapsed" class="lang-select--sidebar lang-segment-row">
    <span class="lang-icon-wrap"><LanguageToggleIcon class="lang-icon" :english="language === 'en'" /></span>
    <SegmentedControl
      class="lang-segments"
      size="small"
      equal
      :model-value="language"
      :options="languageOptions"
      :aria-label="$t('common.switchLanguage')"
      @change="handleSetLanguage"
    />
  </div>
  <div v-else-if="sidebar" class="lang-select--sidebar is-collapsed">
    <t-tooltip :content="switchLanguageLabel" placement="right">
      <button
        type="button"
        class="lang-select-container"
        :aria-label="switchLanguageLabel"
        @click="handleSetLanguage(nextLanguage)"
      >
        <span class="lang-icon-wrap"><LanguageToggleIcon class="lang-icon" :english="language === 'en'" /></span>
      </button>
    </t-tooltip>
  </div>
  <t-dropdown
    v-else
    trigger="click"
    :class="{ 'lang-select--sidebar': sidebar, 'is-collapsed': collapsed }"
    :placement="sidebar ? 'top-left' : 'bottom-right'"
    :popup-props="dropdownPopupProps"
    @click="(item) => handleSetLanguage(item.value)"
  >
    <button
      type="button"
      class="lang-select-container"
      :aria-label="`${$t('common.switchLanguage')}: ${currentLanguage}`"
      :aria-expanded="menuVisible"
      aria-haspopup="menu"
      :title="collapsed ? `${$t('common.switchLanguage')}: ${currentLanguage}` : undefined"
    >
      <span class="lang-icon-wrap"><LanguageToggleIcon class="lang-icon" :english="language === 'en'" /></span>
      <span v-if="!collapsed" class="lang-text">{{ currentLanguage }}</span>
      <ChevronDownIcon v-if="!collapsed" class="lang-chevron" aria-hidden="true" />
    </button>
    <template #dropdown>
      <t-dropdown-menu class="lang-dropdown">
        <t-dropdown-item
          value="zh"
          :active="language === 'zh'"
          class="lang-item"
          :class="{ 'is-selected': language === 'zh' }"
        >
          <div class="lang-item-row">
            <span class="dropdown-item-text">{{ $t('common.lang.zh') }}</span>
            <span v-if="language === 'zh'" class="check-mark">✓</span>
          </div>
        </t-dropdown-item>
        <t-dropdown-item
          value="en"
          :active="language === 'en'"
          class="lang-item"
          :class="{ 'is-selected': language === 'en' }"
        >
          <div class="lang-item-row">
            <span class="dropdown-item-text">{{ $t('common.lang.en') }}</span>
            <span v-if="language === 'en'" class="check-mark">✓</span>
          </div>
        </t-dropdown-item>
      </t-dropdown-menu>
    </template>
  </t-dropdown>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import Cookies from 'js-cookie';
import { LANG_KEY } from '@/locales';
import { ChevronDownIcon } from 'tdesign-icons-vue-next';
import LanguageToggleIcon from './LanguageToggleIcon.vue';
import SegmentedControl from '@/components/SegmentedControl/index.vue';

defineProps({
  sidebar: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
});

const i18n = useI18n();

const language = computed(() => i18n.locale.value);
const currentLanguage = computed(() => i18n.t(`common.lang.${language.value === 'zh' ? 'zh' : 'en'}`));
const nextLanguage = computed(() => language.value === 'zh' ? 'en' : 'zh');
const switchLanguageLabel = computed(() => i18n.t('common.switchToLanguage', {
  language: i18n.t(`common.lang.${nextLanguage.value}`),
}));
const menuVisible = ref(false);
const dropdownPopupProps = {
  overlayClassName: 'lang-dropdown-popper',
  onVisibleChange: (visible) => {
    menuVisible.value = visible;
  },
};
const languageOptions = computed(() => [
  { value: 'zh', label: i18n.t('common.lang.zh'), ariaLabel: i18n.t('common.lang.zh') },
  { value: 'en', label: 'EN', ariaLabel: i18n.t('common.lang.en') },
]);

const handleSetLanguage = (lang) => {
  if (!lang || lang === language.value) return;
  i18n.locale.value = lang;
  Cookies.set(LANG_KEY, lang);
};
</script>

<style lang="scss" scoped>
.lang-select-container {
  cursor: pointer;
  display: flex;
  align-items: center;
  height: 26px;
  padding: 0 8px;
  border-radius: 4px;
  color: #1f2933;
  background: transparent;
  font: inherit;
  transition: background-color 120ms ease, color 120ms ease;
  border: none;
  margin-right: 16px;

  &:hover {
    color: var(--td-brand-color, #2563eb);
    background: transparent;
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color, #2563eb);
    outline-offset: -2px;
  }

  .lang-icon-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    margin-right: 8px;
  }

  .lang-icon {
    font-size: 18px;
  }

  .lang-text {
    font-size: 14px;
    margin-right: 6px;
    user-select: none;
    font-weight: 500;
  }

  .lang-chevron {
    font-size: 14px;
    opacity: 0.7;
  }
}

.lang-select--sidebar {
  display: flex;
  width: 100%;

  .lang-select-container {
    gap: 10px;
    width: 100%;
    height: 44px;
    padding: 0 12px;
    margin: 0;
    border-radius: 8px;
    color: #526477;
    text-align: left;
    white-space: nowrap;
    box-sizing: border-box;

    &:hover,
    &[aria-expanded='true'] {
      background: #eaf0f5;
      color: #203040;
    }
  }

  .lang-icon-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: 0 0 24px;
    width: 24px;
    height: 24px;
    margin: 0;
  }

  .lang-icon {
    font-size: 20px;
  }

  .lang-text {
    flex: 1;
    margin: 0;
    font-weight: 400;
  }

  .lang-chevron {
    flex-shrink: 0;
  }
}

.lang-segment-row {
  align-items: center;
  gap: 10px;
  height: 44px;
  padding: 0 12px;
  color: #526477;
  box-sizing: border-box;
}

.lang-segments {
  flex: 0 0 102px;
}

@media (prefers-reduced-motion: reduce) {
  .lang-select-container {
    transition: none;
  }
}
</style>

<style lang="scss">
// The dropdown is teleported outside this component, so these overrides must stay global.
.lang-dropdown-popper {
  .t-popup__content {
    padding: 6px;
    border-radius: 8px;
    border: 1px solid #e4e7ed;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  }

  .t-dropdown__item {
    padding: 8px 16px;
    border-radius: 4px;
    margin: 2px 0;
    line-height: 1.5;
    font-weight: 400;
    min-width: 120px;
    color: #606266;

    &:hover {
      background-color: #f5f7fa;
      color: var(--td-brand-color, #2563eb);
    }

    &.is-selected,
    &.t-dropdown__item--active {
      color: var(--td-brand-color, #2563eb);
      font-weight: 600;
      background-color: var(--td-brand-color-light, #eff6ff);
    }
  }

  .lang-item-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
  }

  .dropdown-item-text {
    flex: 1;
  }

  .check-mark {
    font-weight: bold;
    font-size: 14px;
  }
}
</style>
