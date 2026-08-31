<template>
  <main class="not-found-page" data-testid="not-found-page">
    <div class="not-found-page__glow not-found-page__glow--top" />
    <div class="not-found-page__glow not-found-page__glow--bottom" />

    <header class="not-found-header">
      <RouterLink
        :to="overviewRoute"
        class="not-found-brand"
        aria-label="HAMi WebUI"
      >
        <img src="@/assets/logo/hami-graph-color.svg" alt="" />
        <span>HAMi WebUI</span>
      </RouterLink>
      <div class="not-found-language">
        <LangSelect />
      </div>
    </header>

    <section class="not-found-content" aria-labelledby="not-found-title">
      <div class="not-found-mark" aria-hidden="true">
        <span>4</span>
        <span class="not-found-mark__logo">
          <img src="@/assets/logo/hami-graph-color.svg" alt="" />
        </span>
        <span>4</span>
      </div>

      <p class="not-found-kicker">HAMi WebUI</p>
      <h1 id="not-found-title">{{ $t('error.404.title') }}</h1>
      <p class="not-found-message">{{ $t('error.404.message') }}</p>
      <p class="not-found-guidance">{{ $t('error.404.guidance') }}</p>

      <div class="not-found-path">
        <span>{{ $t('error.404.requestedPath') }}</span>
        <code>{{ route.fullPath }}</code>
      </div>

      <RouterLink
        :to="overviewRoute"
        class="not-found-action"
        data-testid="not-found-overview-link"
      >
        <span>{{ $t('error.404.backOverview') }}</span>
        <span aria-hidden="true">→</span>
      </RouterLink>
    </section>
  </main>
</template>

<script setup>
import { RouterLink, useRoute } from 'vue-router';

import LangSelect from '@/components/LangSelect/index.vue';

defineOptions({
  name: 'PageNotFound',
});

const route = useRoute();
const overviewRoute = { name: 'overview' };
</script>

<style lang="scss" scoped>
.not-found-page {
  position: relative;
  min-height: 100%;
  width: 100%;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: clamp(20px, 4vw, 48px);
  color: #203040;
  background:
    radial-gradient(circle at 50% 45%, rgb(15 208 93 / 8%), transparent 34%),
    linear-gradient(145deg, #f7faf9 0%, #ffffff 54%, #f2f8f5 100%);
}

.not-found-page__glow {
  position: absolute;
  border-radius: 50%;
  pointer-events: none;
  filter: blur(1px);
}

.not-found-page__glow--top {
  width: min(34vw, 420px);
  aspect-ratio: 1;
  top: -20%;
  right: -7%;
  background: rgb(15 208 93 / 9%);
}

.not-found-page__glow--bottom {
  width: min(42vw, 540px);
  aspect-ratio: 1;
  bottom: -38%;
  left: -12%;
  border: 1px solid rgb(15 208 93 / 14%);
}

.not-found-header {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.not-found-brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  color: #203040;
  font-size: 16px;
  font-weight: 650;
  line-height: 1;
}

.not-found-brand img {
  width: 34px;
  height: 34px;
  flex: none;
}

.not-found-brand:hover {
  color: #0b9f4d;
}

.not-found-language {
  flex: none;
}

.not-found-language :deep(.lang-select-container) {
  margin-right: 0;
}

.not-found-content {
  position: relative;
  z-index: 1;
  width: min(100%, 680px);
  margin: auto;
  padding: clamp(36px, 7vh, 72px) 0;
  text-align: center;
}

.not-found-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: clamp(8px, 2vw, 18px);
  min-height: clamp(92px, 19vw, 150px);
  color: #d6e2dc;
  font-size: clamp(88px, 16vw, 148px);
  font-weight: 720;
  line-height: 0.8;
  letter-spacing: -0.08em;
  user-select: none;
}

.not-found-mark__logo {
  width: clamp(76px, 12vw, 112px);
  height: clamp(76px, 12vw, 112px);
  display: grid;
  place-items: center;
  flex: none;
  border: 1px solid #dce9e2;
  border-radius: 50%;
  background: rgb(255 255 255 / 88%);
  box-shadow: 0 18px 52px rgb(24 84 52 / 10%);
}

.not-found-mark__logo img {
  width: 54%;
  height: 54%;
}

.not-found-kicker {
  margin: clamp(24px, 5vh, 42px) 0 10px;
  color: #0b9f4d;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.not-found-content h1 {
  margin: 0;
  color: #172b3a;
  font-size: clamp(32px, 5vw, 48px);
  font-weight: 680;
  line-height: 1.16;
  letter-spacing: -0.035em;
}

.not-found-message,
.not-found-guidance {
  margin-right: auto;
  margin-left: auto;
  color: #5d6d78;
  font-size: 15px;
  line-height: 1.7;
}

.not-found-message {
  margin-top: 20px;
  margin-bottom: 0;
}

.not-found-guidance {
  margin-top: 0;
  margin-bottom: 24px;
}

.not-found-path {
  width: min(100%, 520px);
  margin: 0 auto 28px;
  padding: 10px 14px;
  border: 1px solid #e2ebe6;
  border-radius: 10px;
  display: flex;
  align-items: baseline;
  justify-content: center;
  gap: 8px;
  color: #7b8992;
  background: rgb(255 255 255 / 72%);
  font-size: 12px;
  line-height: 1.5;
}

.not-found-path code {
  min-width: 0;
  color: #415360;
  overflow-wrap: anywhere;
}

.not-found-action {
  min-height: 42px;
  padding: 0 20px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #ffffff;
  background: #0b9f4d;
  box-shadow: 0 8px 20px rgb(11 159 77 / 18%);
  font-size: 14px;
  font-weight: 600;
  transition:
    background-color 160ms ease,
    box-shadow 160ms ease,
    transform 160ms ease;
}

.not-found-action:hover,
.not-found-action:focus-visible {
  color: #ffffff;
  background: #087f3e;
  box-shadow: 0 10px 24px rgb(11 159 77 / 24%);
  transform: translateY(-1px);
}

.not-found-action:focus-visible,
.not-found-brand:focus-visible {
  outline: 3px solid rgb(15 208 93 / 28%);
  outline-offset: 3px;
}

@media (max-width: 520px) {
  .not-found-page {
    padding: 18px;
  }

  .not-found-header {
    gap: 12px;
  }

  .not-found-brand span {
    display: none;
  }

  .not-found-content {
    padding-top: 32px;
    padding-bottom: 32px;
  }

  .not-found-path {
    align-items: center;
    flex-direction: column;
  }
}

@media (prefers-reduced-motion: reduce) {
  .not-found-action {
    transition: none;
  }
}
</style>
