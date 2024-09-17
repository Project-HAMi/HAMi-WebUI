<template>
  <div class="user-card">
    <div class="top">
      <div>
        <svg-icon icon="home-avatar" class="user-card-avatar" />
      </div>
      <div class="info">
        <div class="username">
          {{ userInfo.username }}
          <el-tag type="warning">{{
            userInfo.detail?.edges.roles.at(-1)?.name
          }}</el-tag>
        </div>
        <div class="email">{{ userInfo.detail?.email }}</div>
      </div>
      <div class="extra"><slot name="extra" /></div>

      <svg-icon icon="block-bg" class="user-card-bg" />
    </div>

    <div class="bottom">
      <slot name="bottom" />
    </div>
  </div>
</template>

<script setup>
import { useStore } from 'vuex';
import { useRouter } from 'vue-router';
import { computed } from 'vue';

const store = useStore();

const userInfo = computed(() => {
  return store.state.user;
});
</script>

<style lang="scss">
.user-card {
  position: relative;

  .top {
    padding: 20px;
    width: 100%;
    height: 88px;
    border-radius: 6px 6px 0px 0px;
    background: linear-gradient(0deg, #fff 0%, #e2eefe 100%);
    display: flex;
    position: relative;

    .user-card-avatar {
      height: 48px !important;
      width: 48px !important;
      z-index: 10;
    }
    .info {
      margin-left: 16px;
      .username {
        color: #1d2b3a;
        font-family: Roboto;
        font-size: 16px;
        font-style: normal;
        font-weight: 500;
        line-height: 24px; /* 150% */
        margin-bottom: 2px;
        display: flex;
        align-items: center;
        gap: 6px;
      }
      z-index: 10;
    }
    .switch {
      position: absolute;
      right: 20px;
      top: 28px;
    }

    .user-card-bg {
      position: absolute;
      right: 0;
      top: -10px;
      height: 75px;
      width: 50%;
      z-index: 0;
    }

    .extra {
      z-index: 10;
    }
  }

  .bottom {
    padding: 20px;
    padding-top: 0;

    .user-items {
      display: flex;
      li {
        display: flex;
        padding: 8px 0px;
        flex-direction: column;
        align-items: center;
        gap: 8px;
        flex: 1 0 0;
        &:hover {
          color: var(--el-color-primary) !important;
          cursor: pointer;
        }
        .count {
          font-family: Roboto;
          font-size: 20px;
          font-style: normal;
          font-weight: 700;
          line-height: 100%; /* 20px */
        }
      }
    }
  }
}
</style>
