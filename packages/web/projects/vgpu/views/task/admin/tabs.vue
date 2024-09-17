<script setup>
defineProps(['config']);
</script>

<template>
  <div class="tabs-container">
    <div class="tabs">
      <template v-for="({ tab }, index) in config">
        <input type="radio" :id="`radio-${index + 1}`" name="tabs" checked="" />
        <label class="tab" :for="`radio-${index + 1}`">{{ tab }}</label>
      </template>

      <!--      <input type="radio" id="radio-2" name="tabs" />-->
      <!--      <label class="tab" for="radio-2">UI</label>-->
      <!--      <input type="radio" id="radio-3" name="tabs" />-->
      <!--      <label class="tab" for="radio-3">World</label>-->
      <span class="glider"></span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.tabs {
  display: flex;
  position: relative;
  background-color: #fff;
  box-shadow: 0 0 1px 0 rgba(24, 94, 224, 0.15),
    0 6px 12px 0 rgba(24, 94, 224, 0.15);
  //padding: 0.75rem;
  padding: 2px;
  border-radius: 8px;
}

.tabs * {
  z-index: 2;
}

.tabs-container input[type='radio'] {
  display: none;
}

.tab {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 30px;
  width: 50px;
  font-size: 0.8rem;
  color: black;
  font-weight: 500;
  border-radius: 8px;
  cursor: pointer;
  transition: color 0.15s ease-in;
}

.notification {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 0.8rem;
  height: 0.8rem;
  position: absolute;
  top: 10px;
  left: 30%;
  font-size: 10px;
  margin-left: 0.75rem;
  border-radius: 50%;
  margin: 0px;
  background-color: #e6eef9;
  transition: 0.15s ease-in;
}

.tabs-container input[type='radio']:checked + label {
  color: #185ee0;
}

.tabs-container input[type='radio']:checked + label > .notification {
  background-color: #185ee0;
  color: #fff;
  margin: 0px;
}

.tabs-container input[id='radio-1']:checked ~ .glider {
  transform: translateX(0);
}

.tabs-container input[id='radio-2']:checked ~ .glider {
  transform: translateX(100%);
}

.tabs-container input[id='radio-3']:checked ~ .glider {
  transform: translateX(200%);
}

.glider {
  position: absolute;
  display: flex;
  height: 30px;
  width: 50px;
  background-color: #e6eef9;
  z-index: 1;
  border-radius: 8px;
  transition: 0.25s ease-out;
}
</style>
