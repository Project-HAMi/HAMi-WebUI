<template>
  <Block
    :title="title"
    class="resource-summary resource-switcher"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
    @focusin="handleFocusIn"
  >
    <template #extra>
      <button
        ref="rotationControl"
        type="button"
        role="switch"
        class="resource-rotation"
        :aria-checked="autoRotate"
        @click="autoRotate = !autoRotate"
      >
        <span>{{ labels.autoRotate }}</span>
        <span class="resource-rotation__track" aria-hidden="true"><span /></span>
      </button>
    </template>

    <div
      ref="tabList"
      class="resource-tabs"
      role="tablist"
      :aria-label="labels.chooseResource"
    >
      <button
        v-for="(resource, index) in resources"
        :id="tabId(resource.id)"
        :key="resource.id"
        :ref="(element) => setTabElement(resource.id, element)"
        type="button"
        role="tab"
        class="resource-tab"
        :class="{ 'is-selected': activeId === resource.id }"
        :aria-selected="activeId === resource.id"
        :aria-controls="panelId(resource.id)"
        :aria-busy="resource.status === 'loading'"
        :tabindex="activeId === resource.id ? 0 : -1"
        @click="selectResource(resource.id)"
        @keydown="handleTabKey($event, index)"
      >
        <LoadingValue v-if="resource.status === 'loading'" :height="30" width="48px" />
        <span v-else class="resource-tab__count">{{ resource.status === 'ready' ? (resource.count ?? '—') : '—' }}</span>
        <span class="resource-tab__label">{{ resource.title }}</span>
        <span v-if="resource.status !== 'ready'" class="resource-sr-only">{{ stateText(resource.status) }}</span>
      </button>
      <span
        v-show="indicatorX !== null"
        class="resource-tabs__indicator"
        :style="{ transform: `translateX(${indicatorX ?? 0}px)` }"
        aria-hidden="true"
      ><span ref="indicatorProgress" /></span>
    </div>

    <div class="resource-panels">
      <section
        v-for="resource in resources"
        :id="panelId(resource.id)"
        :key="resource.id"
        role="tabpanel"
        class="resource-panel"
        :class="{ 'is-active': activeId === resource.id }"
        :inert="activeId !== resource.id"
        :aria-hidden="activeId !== resource.id"
        :aria-labelledby="tabId(resource.id)"
        :aria-busy="resource.panelStatus === 'loading'"
        :aria-live="autoRotate ? 'off' : 'polite'"
        :tabindex="activeId === resource.id ? 0 : -1"
      >
        <div class="resource-panel__header">
          <div class="resource-panel__heading">
            <h3>{{ resource.panelTitle }}</h3>
            <MetricHelp
              v-if="resource.description"
              :description="resource.description"
              :help-label="labels.helpLabel"
            />
          </div>
          <OverviewAllLink v-if="resource.allTo" :to="resource.allTo" :label="resource.allLabel" />
        </div>

        <div
          v-if="resource.panelStatus === 'loading'"
          class="resource-statuses resource-statuses--loading"
          aria-hidden="true"
        >
          <div v-for="index in 2" :key="index" class="resource-status">
            <LoadingValue :height="16" width="60%" />
            <LoadingValue :height="24" width="32px" />
          </div>
        </div>
        <div v-else-if="resource.panelStatus !== 'ready'" class="resource-panel__state">
          {{ stateText(resource.panelStatus) }}
        </div>
        <div
          v-else
          class="resource-statuses"
          :style="{ '--status-count': resource.items.length }"
        >
          <div
            v-for="item in resource.items"
            :key="item.id"
            class="resource-status"
            :class="[`resource-status--${item.tone || 'neutral'}`, { 'is-link': !!item.to, 'has-help': !!item.description }]"
          >
            <component
              :is="item.to ? RouterLink : 'div'"
              :to="item.to || undefined"
              class="resource-status__main"
              :aria-label="item.to ? `${item.label}: ${item.count ?? '—'}` : undefined"
            >
              <span class="resource-status__label">{{ item.label }}</span>
              <span class="resource-status__count">{{ item.count ?? '—' }}</span>
            </component>
            <span v-if="item.description" class="resource-status__help">
              <MetricHelp
                :description="item.description"
                :help-label="item.helpLabel || labels.helpLabel"
              />
            </span>
          </div>
        </div>
        <span v-if="resource.panelStatus === 'loading'" class="resource-sr-only">{{ labels.loading }}</span>
      </section>
    </div>
  </Block>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue';
import { RouterLink } from 'vue-router';
import Block from './Block.vue';
import LoadingValue from '~/vgpu/components/LoadingValue.vue';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import OverviewAllLink from './OverviewAllLink.vue';

const props = defineProps({
  title: { type: String, required: true },
  resources: { type: Array, required: true },
  labels: { type: Object, required: true },
});

const componentId = useId();
const activeId = ref(props.resources.find((resource) => resource.id === 'nodes')?.id || props.resources[0]?.id);
const autoRotate = ref(false);
const hovered = ref(false);
const hidden = ref(false);
const rotationControl = ref(null);
const tabList = ref(null);
const indicatorX = ref(null);
const indicatorProgress = ref(null);
const tabElements = new Map();
let tabResizeObserver;
const tabId = (id) => `${componentId}-resource-tab-${id}`;
const panelId = (id) => `${componentId}-resource-panel-${id}`;
const stateText = (status) => props.labels[status] || props.labels.missing;
const updateIndicator = () => {
  const element = tabElements.get(activeId.value);
  if (!element) {
    indicatorX.value = null;
    return;
  }
  const style = getComputedStyle(element);
  const paddingLeft = parseFloat(style.paddingLeft) || 0;
  indicatorX.value = element.offsetLeft + paddingLeft;
};
const setTabElement = (id, element) => {
  const previous = tabElements.get(id);
  if (previous === element) return;
  if (previous) tabResizeObserver?.unobserve(previous);
  if (element) tabElements.set(id, element);
  else tabElements.delete(id);
  if (element) tabResizeObserver?.observe(element);
};
const selectResource = (id) => {
  autoRotate.value = false;
  activeId.value = id;
};

const handleTabKey = async (event, index) => {
  const last = props.resources.length - 1;
  const target = {
    ArrowLeft: index === 0 ? last : index - 1,
    ArrowRight: index === last ? 0 : index + 1,
    Home: 0,
    End: last,
  }[event.key];
  if (target === undefined) return;
  event.preventDefault();
  const id = props.resources[target]?.id;
  if (id === undefined) return;
  selectResource(id);
  await nextTick();
  tabElements.get(id)?.focus();
};

const handleFocusIn = (event) => {
  // The rotation switch is the explicit opt-in; entering the data stops it.
  if (!rotationControl.value?.contains(event.target)) autoRotate.value = false;
};

const canRotate = computed(() => autoRotate.value && !hovered.value && !hidden.value && props.resources.length > 1);
const rotationDuration = 8000;
let rotationAnimation;
let animationResourceId;
let reducedMotionQuery;
let disposed = false;
const clearRotation = () => {
  if (rotationAnimation) {
    rotationAnimation.onfinish = null;
    rotationAnimation.cancel();
  }
  rotationAnimation = undefined;
  animationResourceId = undefined;
};
const advanceRotation = () => {
  clearRotation();
  const current = props.resources.findIndex((resource) => resource.id === activeId.value);
  activeId.value = props.resources[(current + 1) % props.resources.length].id;
};
const rotationEasing = () => (reducedMotionQuery?.matches ? 'steps(8, end)' : 'linear');
const syncRotation = () => {
  if (disposed || !autoRotate.value || props.resources.length < 2 || !indicatorProgress.value) {
    clearRotation();
    return;
  }
  if (animationResourceId !== activeId.value) clearRotation();
  if (!rotationAnimation) {
    // One animation owns both the visible countdown and the next-panel event.
    const animation = indicatorProgress.value.animate(
      [{ transform: 'scaleX(1)' }, { transform: 'scaleX(0)' }],
      { duration: rotationDuration, easing: rotationEasing(), fill: 'forwards' },
    );
    rotationAnimation = animation;
    animationResourceId = activeId.value;
    animation.onfinish = () => {
      if (rotationAnimation === animation && canRotate.value && !disposed) advanceRotation();
    };
  }
  if (!canRotate.value) {
    rotationAnimation.pause();
  } else if (Number(rotationAnimation.currentTime) >= rotationDuration) {
    // A pause at the exact deadline keeps the completed interval until resume.
    advanceRotation();
  } else if (rotationAnimation.playState !== 'running') {
    rotationAnimation.play();
  }
};
watch([autoRotate, canRotate], syncRotation, { flush: 'post' });
watch(activeId, () => {
  updateIndicator();
  syncRotation();
}, { flush: 'post' });
watch(() => props.resources.map((resource) => resource.id).join('|'), () => {
  if (!props.resources.some((resource) => resource.id === activeId.value)) activeId.value = props.resources[0]?.id;
  nextTick(() => {
    updateIndicator();
    syncRotation();
  });
});
const handleVisibilityChange = () => { hidden.value = document.hidden; };
const handleReducedMotionChange = () => {
  rotationAnimation?.effect?.updateTiming({ easing: rotationEasing() });
};
onMounted(() => {
  handleVisibilityChange();
  document.addEventListener('visibilitychange', handleVisibilityChange);
  reducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
  reducedMotionQuery.addEventListener('change', handleReducedMotionChange);
  tabResizeObserver = new ResizeObserver(updateIndicator);
  if (tabList.value) tabResizeObserver.observe(tabList.value);
  tabElements.forEach((element) => tabResizeObserver.observe(element));
  updateIndicator();
});
onBeforeUnmount(() => {
  disposed = true;
  clearRotation();
  tabResizeObserver?.disconnect();
  reducedMotionQuery?.removeEventListener('change', handleReducedMotionChange);
  document.removeEventListener('visibilitychange', handleVisibilityChange);
});
</script>

<style scoped lang="scss">
.resource-switcher {
  min-width: 0;

  :deep(.home-block-header) {
    gap: 12px;
  }
}

.resource-rotation {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 24px;
  padding: 0;
  border: 0;
  background: transparent;
  color: #697886;
  font: inherit;
  font-size: 12px;
  white-space: nowrap;
  cursor: pointer;

  &__track {
    width: 28px;
    height: 16px;
    padding: 2px;
    border-radius: 16px;
    background: #bcc5cf;
    box-sizing: border-box;
    transition: background 120ms;

    > span {
      display: block;
      width: 12px;
      height: 12px;
      border-radius: 50%;
      background: #fff;
      transition: transform 120ms;
    }
  }

  &[aria-checked='true'] &__track {
    background: var(--el-color-primary);

    > span { transform: translateX(12px); }
  }
}

.resource-tabs {
  position: relative;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;

  &__indicator {
    position: absolute;
    bottom: 0;
    left: 0;
    width: 40px;
    height: 2px;
    pointer-events: none;
    transition: transform 200ms cubic-bezier(.2, .8, .2, 1);

    > span {
      display: block;
      width: 100%;
      height: 100%;
      border-radius: 2px;
      background: var(--el-color-primary);
      transform-origin: left center;
    }
  }
}

.resource-tab {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 5px;
  min-width: 0;
  min-height: 78px;
  padding: 10px 12px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: #1d2b3a;
  text-align: left;
  font: inherit;
  cursor: pointer;
  transition: background-color 200ms cubic-bezier(.2, .8, .2, 1);

  &:hover { background: #f8fafc; }

  &__count {
    color: #1d2b3a;
    font-family: Roboto, -apple-system, BlinkMacSystemFont, sans-serif;
    font-size: 24px;
    line-height: 30px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }

  &__label {
    font-size: 12px;
    line-height: 18px;
    color: #697886;
    overflow-wrap: anywhere;
  }

  &.is-selected &__label {
    color: #1d2b3a;
    font-weight: 600;
  }
}

.resource-panels {
  display: grid;
}

.resource-panel {
  grid-area: 1 / 1;
  min-width: 0;
  margin-top: 16px;
  visibility: hidden;

  &.is-active { visibility: visible; }

  &__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    min-height: 20px;
    margin-bottom: 10px;
  }

  &__heading {
    display: flex;
    align-items: center;
    gap: 2px;
    min-width: 0;

    h3 {
      margin: 0;
      color: #697886;
      font-size: 13px;
      font-weight: 500;
      line-height: 20px;
    }
  }

  &__state {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 48px;
    color: #939ea9;
    font-size: 12px;
    text-align: center;
  }
}

.resource-statuses {
  display: grid;
  grid-template-columns: repeat(var(--status-count, 3), minmax(0, 1fr));
  grid-auto-rows: minmax(0, 1fr);
  gap: 8px;
  min-height: 48px;

  &--two,
  &--loading { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

.resource-status {
  position: relative;
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  border-radius: 6px;
  background: #f5f7fa;

  &__main {
    display: flex;
    flex: 1;
    align-items: center;
    align-self: stretch;
    justify-content: space-between;
    gap: 8px;
    min-width: 0;
    min-height: 48px;
    padding: 12px;
    box-sizing: border-box;
    color: inherit;
    text-decoration: none;
    border-radius: inherit;
  }

  &__label {
    min-width: 0;
    color: #324558;
    font-size: 12px;
    line-height: 18px;
  }

  &__count {
    flex: 0 0 auto;
    font-family: Roboto, -apple-system, BlinkMacSystemFont, sans-serif;
    color: #1d2b3a;
    font-size: 22px;
    font-weight: 600;
    line-height: 24px;
    font-variant-numeric: tabular-nums;
  }

  &--success &__count { color: #16a34a; }
  &--warning &__count { color: #b86b08; }
  &--danger &__count { color: #d94b4b; }

  &.is-link:hover { background: #edf2fa; }

  &.has-help &__main { padding-right: 0; }

  &__help {
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    margin-right: 9px;
  }
}

.resource-statuses--loading .resource-status {
  gap: 8px;
  justify-content: space-between;
  padding: 12px;
}

.resource-tab:focus-visible,
.resource-rotation:focus-visible,
.resource-panel:focus-visible,
.resource-status__main:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
  border-radius: 4px;
}

.resource-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (prefers-reduced-motion: reduce) {
  .resource-tab,
  .resource-tabs__indicator,
  .resource-rotation__track,
  .resource-rotation__track > span { transition: none; }
}
</style>
