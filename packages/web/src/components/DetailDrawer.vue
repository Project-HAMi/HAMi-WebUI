<template>
  <div @click="visible = true" class="detailDrawerFlag">
    <slot />
  </div>
  <el-drawer
    v-model="visible"
    :append-to-body="true"
    destroy-on-close
    direction="btt"
    size="60%"
    id="detailDrawer"
    :with-header="false"
  >
    <ul class="content">
      <li :key="title" class="item" v-for="{ title, children } in content">
        <el-descriptions :title="title" :column="1">
          <el-descriptions-item
            :key="v.label"
            v-for="v in children"
            :label="v.label"
          >
            <component v-if="isVNode(v.value)" :is="v.value"></component>
            <span v-else>{{ v.value }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </li>
    </ul>
  </el-drawer>
</template>

<script setup lang="jsx">
import { ref, defineProps, isVNode } from 'vue';

const props = defineProps({
  content: {
    type: Array,
    default: () => [],
  },
});

const visible = ref(false);
</script>

<style lang="scss">
.detailDrawerFlag {
  cursor: pointer;
  color: var(--el-color-primary);
}

#detailDrawer {
  .el-descriptions__body {
    background-color: transparent;
  }
  .content {
    list-style: none;
    display: flex;
    height: 100%;
    .item {
      flex: 1;
      background-color: #eee;
      padding: 15px;
      border-radius: 10px;
      overflow: auto;
      .el-table {
        background-color: transparent;
      }
    }
  }
}
</style>
