<template>
  <a class="navbar__brand" href="/zh/">
    <div class="navbar__logo">
      <img
        src="@/assets/logo/hami-graph-color.svg"
        alt="HAMi"
        class="navbar__icon"
      />
    </div>
    <b class="navbar__title text--truncate">HAMi</b>
  </a>
</template>

<style>
.navbar__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  color: inherit;
}

.navbar__brand:hover {
  color: inherit;
  text-decoration: none;
}

.navbar__logo {
  display: flex;
  align-items: center;
}

.navbar__logo img {
  width: 32px;
  height: 32px;
  display: block;
}

.navbar__title {
  font-family: -apple-system, BlinkMacSystemFont, var(--td-font-family);
  font-size: 16px;
  font-style: normal;
  font-weight: 600;
  line-height: 20px;
  color: #203040;
}

.navbar__brand:hover .navbar__title {
  color: #203040;
}

.text--truncate {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
