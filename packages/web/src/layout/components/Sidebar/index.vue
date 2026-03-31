<template>
  <div class="sidebar">
    <div :class="['side-container', collapsed && 'is-collapsed']">
      <div class="side-logo">
        <div class="side-logo-main" @click="router.push('/')">
          <div v-show="!collapsed" class="logo-expanded">
            <Logo />
          </div>
          <img
            v-show="collapsed"
            src="@/assets/logo/hami-graph-color.svg"
            alt="HAMi"
            class="logo-compact-graph"
          />
        </div>
      </div>

      <div class="side-menus">
        <t-menu
          :width="menuWidth"
          :value="activeValue"
          v-model:expanded="menuExpanded"
          :collapsed="collapsed"
          theme="light"
          expand-type="normal"
        >
          <template v-for="menuRoute in menuRoutes" :key="menuRoute.path">
            <t-submenu
              v-if="(menuRoute.children || []).some((c) => c.meta)"
              :value="menuRoute.path"
              :class="{
                'resource-submenu': menuRoute.name === 'resource-admin',
                'resource-submenu-child-active':
                  menuRoute.name === 'resource-admin' && resourceMenuChildActive,
              }"
              :title="$t(menuRoute.meta?.title)"
            >
              <template #icon>
                <svg-icon
                  v-if="menuRoute.name === 'resource-admin'"
                  icon="resource"
                  class-name="t-icon side-resource-icon"
                />
              </template>
              <t-menu-item
                v-for="child in (menuRoute.children || []).filter((c) => c.meta)"
                :key="child.path"
                :value="child.path"
                @click="router.push(child.path)"
              >
                {{ $t(child.meta?.title) }}
              </t-menu-item>
            </t-submenu>

            <t-menu-item
              v-else
              :value="menuRoute.path"
              @click="router.push(menuRoute.path)"
            >
              {{ $t(menuRoute.meta?.title) }}
            </t-menu-item>
          </template>
        </t-menu>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Logo from '@/layout/components/TopBar/logo.vue';

defineProps({
  collapsed: {
    type: Boolean,
    default: false,
  },
  menuWidth: {
    type: Array,
    default: () => ['200px', '72px'],
  },
});

const route = useRoute();
const router = useRouter();

const menuRoutes = computed(
  () => route.matched?.[0]?.children?.filter((item) => item.meta) ?? [],
);

const leafRoutes = computed(() =>
  menuRoutes.value.flatMap((menuRoute) =>
    (menuRoute.children || []).filter((child) => child.meta),
  ),
);

const activeValue = computed(() => {
  const currentPath = route.path;
  const matches = leafRoutes.value.filter(
    (leaf) => currentPath === leaf.path || currentPath.includes(leaf.path),
  );
  if (matches.length === 0) return currentPath;
  matches.sort((a, b) => b.path.length - a.path.length);
  return matches[0].path;
});

const resourceMenuChildActive = computed(() => {
  const resourceRoute = menuRoutes.value.find(
    (menuRoute) => menuRoute.name === 'resource-admin',
  );
  if (!resourceRoute) return false;
  const children = (resourceRoute.children || []).filter((child) => child.meta);
  return children.some((child) => child.path === activeValue.value);
});

const expandedKeysFromRoute = computed(() => {
  const active = activeValue.value;
  return menuRoutes.value
    .filter((menuRoute) => {
      const children = (menuRoute.children || []).filter((c) => c.meta);
      return children.some((child) => child.path === active);
    })
    .map((menuRoute) => menuRoute.path);
});

const menuExpanded = ref([]);

watch(
  () => expandedKeysFromRoute.value.slice().sort().join('\0'),
  () => {
    const keys = expandedKeysFromRoute.value;
    if (keys.length === 0) return;
    const next = new Set(menuExpanded.value);
    keys.forEach((k) => next.add(k));
    menuExpanded.value = [...next];
  },
  { immediate: true },
);
</script>

<style lang="scss" scoped>
@import './menu.scss';
</style>
