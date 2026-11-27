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
  <div
    v-else
    ref="dropdownRoot"
    class="lang-dropdown-wrap"
    :class="{ 'lang-select--sidebar': sidebar, 'is-collapsed': collapsed }"
    @focusout="onDropdownFocusout"
  >
    <button
      ref="triggerButton"
      type="button"
      class="lang-select-container"
      :aria-label="`${$t('common.switchLanguage')}: ${currentLanguage}`"
      :aria-expanded="menuVisible"
      aria-haspopup="menu"
      :title="collapsed ? `${$t('common.switchLanguage')}: ${currentLanguage}` : undefined"
      @click="toggleMenu"
      @keydown="onTriggerKeydown"
    >
      <span class="lang-icon-wrap"><LanguageToggleIcon class="lang-icon" :english="language === 'en'" /></span>
      <span v-if="!collapsed" class="lang-text">{{ currentLanguage }}</span>
      <ChevronDownIcon v-if="!collapsed" class="lang-chevron" aria-hidden="true" />
    </button>
    <div
      v-if="menuVisible"
      ref="menuElement"
      role="menu"
      :aria-label="$t('common.switchLanguage')"
      class="lang-dropdown-popper lang-dropdown"
      @keydown="onMenuKeydown"
    >
      <button
        v-for="(option, index) in menuOptions"
        :key="option.value"
        type="button"
        role="menuitemradio"
        :aria-checked="language === option.value"
        :tabindex="activeIndex === index ? 0 : -1"
        class="lang-item"
        :class="{ 'is-selected': language === option.value }"
        @click="selectMenuLanguage(option.value)"
      >
        <span class="lang-item-row">
          <span class="dropdown-item-text">{{ option.label }}</span>
          <span v-if="language === option.value" class="check-mark" aria-hidden="true">✓</span>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
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
const dropdownRoot = ref(null);
const triggerButton = ref(null);
const menuElement = ref(null);
const menuVisible = ref(false);
const activeIndex = ref(0);
const menuOptions = computed(() => [
  { value: 'zh', label: i18n.t('common.lang.zh') },
  { value: 'en', label: i18n.t('common.lang.en') },
]);
const languageOptions = computed(() => [
  { value: 'zh', label: i18n.t('common.lang.zh'), ariaLabel: i18n.t('common.lang.zh') },
  { value: 'en', label: 'EN', ariaLabel: i18n.t('common.lang.en') },
]);

const handleSetLanguage = (lang) => {
  if (!lang || lang === language.value) return;
  i18n.locale.value = lang;
  Cookies.set(LANG_KEY, lang);
};

const focusMenuItem = async (index) => {
  const count = menuOptions.value.length;
  activeIndex.value = ((index % count) + count) % count;
  await nextTick();
  menuElement.value?.querySelectorAll('[role="menuitemradio"]')?.[activeIndex.value]?.focus();
};

const closeMenu = async (restoreFocus = false) => {
  if (!menuVisible.value) return;
  menuVisible.value = false;
  if (restoreFocus) {
    await nextTick();
    triggerButton.value?.focus();
  }
};

const openMenu = (startIndex) => {
  const currentIdx = menuOptions.value.findIndex((option) => option.value === language.value);
  menuVisible.value = true;
  focusMenuItem(startIndex ?? (currentIdx >= 0 ? currentIdx : 0));
};

const toggleMenu = () => {
  if (menuVisible.value) {
    closeMenu();
    return;
  }
  openMenu();
};

const selectMenuLanguage = (lang) => {
  handleSetLanguage(lang);
  closeMenu(true);
};

const onTriggerKeydown = (event) => {
  if (event.key === 'ArrowDown') {
    event.preventDefault();
    openMenu(0);
  } else if (event.key === 'ArrowUp') {
    event.preventDefault();
    openMenu(menuOptions.value.length - 1);
  } else if (event.key === 'Escape' && menuVisible.value) {
    event.preventDefault();
    closeMenu(true);
  }
};

const onMenuKeydown = (event) => {
  if (event.key === 'ArrowDown') {
    event.preventDefault();
    focusMenuItem(activeIndex.value + 1);
  } else if (event.key === 'ArrowUp') {
    event.preventDefault();
    focusMenuItem(activeIndex.value - 1);
  } else if (event.key === 'Home') {
    event.preventDefault();
    focusMenuItem(0);
  } else if (event.key === 'End') {
    event.preventDefault();
    focusMenuItem(menuOptions.value.length - 1);
  } else if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    closeMenu(true);
  } else if (event.key === 'Tab') {
    closeMenu();
  }
};

const onDropdownFocusout = (event) => {
  if (menuVisible.value && event.relatedTarget && !dropdownRoot.value?.contains(event.relatedTarget)) {
    menuVisible.value = false;
  }
};

const onDocumentPointerDown = (event) => {
  if (menuVisible.value && !dropdownRoot.value?.contains(event.target)) {
    menuVisible.value = false;
  }
};

onMounted(() => document.addEventListener('pointerdown', onDocumentPointerDown));
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocumentPointerDown));
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

.lang-dropdown-wrap {
  position: relative;
  display: inline-flex;
}

.lang-dropdown-popper {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 2500;
  min-width: 120px;
  padding: 6px;
  border-radius: 8px;
  border: 1px solid #e4e7ed;
  background: #fff;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.lang-item {
  display: block;
  width: 100%;
  border: 0;
  background: transparent;
  padding: 8px 16px;
  border-radius: 4px;
  margin: 2px 0;
  line-height: 1.5;
  font: inherit;
  font-weight: 400;
  text-align: left;
  color: #606266;
  cursor: pointer;

  &:hover {
    background-color: #f5f7fa;
    color: var(--td-brand-color, #2563eb);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color, #2563eb);
    outline-offset: -2px;
  }

  &.is-selected {
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

@media (prefers-reduced-motion: reduce) {
  .lang-select-container {
    transition: none;
  }
}
</style>
