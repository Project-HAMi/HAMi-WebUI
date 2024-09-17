<template>
  <div
    :class="[
      'sidebar',
      isExpand && 'is-expand',
      hasAnimation && 'hasAnimation',
    ]"
    ref="sideRef"
  >
    <div :class="['side-container', isExpand && 'is-expand']">
      <el-tooltip
        placement="right"
        effect="dark"
        :content="route.matched[0].meta.title"
        :disabled="!isExpand"
        :show-after="500"
      >
        <div class="side-title">
          <svg-icon :icon="route.matched[0].meta.icon" />
          <span v-if="sideWidth >= 200">{{ route.matched[0].meta.title }}</span>
        </div>
      </el-tooltip>

      <ul class="side-menus">
        <li class="side-menus-item" v-for="{ meta, children } in menus">
          <div v-if="sideWidth >= 200" class="side-menus-item-title">
            {{ meta.title }}
          </div>

          <ul class="side-menus-item-children">
            <el-tooltip
              v-for="child in children.filter((v) => v.meta)"
              placement="right"
              effect="dark"
              :content="child.meta?.title"
              :disabled="!isExpand"
              :show-after="500"
            >
              <li
                :class="[
                  'side-menus-item-children-item',
                  { 'is-active': route.path.includes(child.path) },
                ]"
                @click="router.push(child.path)"
              >
                <svg-icon :icon="child.meta?.icon" />
                <span v-if="sideWidth >= 200"> {{ child.meta?.title }}</span>
              </li>
            </el-tooltip>
          </ul>
        </li>
      </ul>

      <div class="expand-btn" @click="handleExpand">
        <svg-icon :icon="isExpand ? 'expand-right' : 'expand-left'" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, nextTick, onBeforeUnmount } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useStore } from 'vuex';

const route = useRoute();
const router = useRouter();

const hasAnimation = ref(false);

const sideWidth = ref(0);

const sideRef = ref();

const resizeObserver = new ResizeObserver((entries) => {
  for (let entry of entries) {
    // 在这里执行你的逻辑
    sideWidth.value = entry.contentRect.width;
  }
});

onMounted(() => {
  resizeObserver.observe(sideRef.value);
});

onBeforeUnmount(() => {
  resizeObserver.disconnect();
});

const store = useStore();

const menus = computed(() =>
  route.matched[0].children.filter((item) => item.meta),
);

const isExpand = ref(false);

const handleExpand = () => {
  hasAnimation.value = true;

  isExpand.value = !isExpand.value;
};

onMounted(async () => {
  await nextTick();
});
</script>

<style lang="scss" scoped>
@keyframes menuExpandLeft {
  from {
    width: 200px;
  }
  to {
    width: 48px;
  }
}

@keyframes menuExpandRight {
  from {
    width: 40px;
  }
  to {
    width: 200px;
  }
}

.hasAnimation {
  .sidebar {
    animation: menuExpandRight 0.4s;
  }
  .is-expand {
    animation: menuExpandLeft 0.4s;
  }
}

.sidebar {
  width: 200px;
  box-sizing: border-box;
  // transition: all 0.4s;
  //animation: menuExpandRight 0.4s;
  //padding-bottom: 80px;

  .side-container {
    animation: menuExpandRight 0.4s;

    position: relative;
    width: 200px;
    height: 100%;
    border-radius: 8px;
    padding: 8px;
    border: 1px solid #fff;
    background: #f5f7fa;

    /* box-shadow/300 */
    box-shadow: 0px 4px 24px 0px rgba(2, 5, 8, 0.04),
      0px 20px 48px 0px rgba(2, 5, 8, 0.08);
    .side-title {
      display: flex;
      padding: 12px 16px;
      align-items: center;
      gap: 8px;
      border-radius: 6px;
      background: linear-gradient(180deg, #324558 0%, #1d2b3a 100%);
      box-shadow: 0px 2px 8px 0px rgba(2, 5, 8, 0.04),
        0px 6px 20px 0px rgba(2, 5, 8, 0.08);
      font-family: Roboto;
      font-size: 14px;
      font-style: normal;
      font-weight: 500;
      color: #fff;
    }

    .side-menus {
      list-style: none;
      padding: 0;
      margin-bottom: 0;
      height: calc(100% - 90px);
      //padding-bottom: 50px;
      overflow: auto;
      &-item {
        margin-top: 16px;
        &-title {
          color: #697886;
          padding: 0px 4px;
          text-overflow: ellipsis;
          /* 中文/正文/小 */
          font-family: 'PingFang SC';
          font-size: 12px;
          font-style: normal;
          font-weight: 400;
          line-height: 20px; /* 166.667% */
          margin-bottom: 4px;
        }
        &-children {
          list-style: none;
          padding: 0;
          &-item {
            display: flex;
            // width: 176px;
            padding: 9px 12px;
            align-items: center;
            gap: 8px;
            color: #324558;
            text-overflow: ellipsis;
            font-family: 'PingFang SC';
            font-size: 13px;
            font-style: normal;
            font-weight: 400;
            line-height: 22px; /* 169.231% */
            cursor: pointer;
            margin-bottom: 4px;
            &:hover {
              border-radius: 6px;
              background: #fff;

              box-shadow: 0px 1px 1px 0px rgba(2, 5, 8, 0.02),
                0px 1px 4px 0px rgba(2, 5, 8, 0.06);
            }
          }

          .is-active {
            border-radius: 6px;
            background: #fff;

            box-shadow: 0px 1px 1px 0px rgba(2, 5, 8, 0.02),
              0px 1px 4px 0px rgba(2, 5, 8, 0.06);
            color: var(--light-color-brand-default, #2563eb);
            text-overflow: ellipsis;
            font-family: 'PingFang SC';
            font-size: 13px;
            font-style: normal;
            font-weight: 500;
            line-height: 22px; /* 169.231% */
          }
        }
      }
    }

    .expand-btn {
      display: flex;
      padding: 12px;
      justify-content: center;
      align-items: center;
      gap: 8px;
      border-radius: 6px;
      background: #fff;
      box-shadow: 0px 1px 1px 0px rgba(2, 5, 8, 0.02),
        0px 1px 4px 0px rgba(2, 5, 8, 0.06);
      cursor: pointer;
      position: absolute;
      bottom: 8px;
      width: calc(100% - 16px);
    }
  }
}

.is-expand {
  width: 48px;
  //animation: menuExpandLeft 0.4s;

  .side-container {
    width: 48px;
    padding: 4px;
    animation: menuExpandLeft 0.4s;
  }
  .side-title {
    height: 40px;
    padding: 0 !important;
    justify-content: center;
  }
  .side-menus-item-children-item {
    height: 32px;
  }

  .expand-btn {
    height: 40px;
    left: 4px;
    width: calc(100% - 8px) !important;
    padding: 0;
  }
}
</style>
