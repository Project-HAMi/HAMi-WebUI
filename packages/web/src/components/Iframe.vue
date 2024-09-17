<template>
    <iframe
      id="iframe"
      :src="src"
      frameborder="0"
      :scrolling="scrolling"
      :allowtransparency="allowtransparency"
      :width="width"
      :height="height"
      @load="onIframeLoad"
    />
  </template>

  <script setup>
  import { onMounted, computed, toRefs, defineProps } from 'vue';
  import iframeResize from "iframe-resizer/js/iframeResizer"

  const props = defineProps({
    src: {
      type: String,
      required: true
    },
    width: {
      type: String,
      default: '100%'
    },
    height: {
      type: String,
      default: '100%'
    },
    scrolling: {
      type: String,
      default: 'auto'
    },
    allowtransparency: {
      type: String,
      default: 'yes'
    }
  });

  const { height, scrolling } = toRefs(props);

  const iframeHeight = computed(() => {
    if (scrolling.value !== 'no') {
      return `${document.body.clientHeight - 84}px`;
    }
    return height.value;
  });

  const onIframeLoad = () => {
    if (scrolling.value === 'no') {
        iframeResize(
        {
          log: true,
          autoResize: true,
          onResize: function (iFrameData) {},
          heightCalculationMethod: 'lowestElement'
        },
        '#iframe'
      );
    }
  };

  onMounted(() => {
    if (scrolling.value !== 'no') {
      document.getElementById('iframe').style.height = iframeHeight.value;
    }
  });
  </script>
