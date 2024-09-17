<template>
  <div id="bg">
    <canvas></canvas>
    <canvas></canvas>
    <canvas></canvas>
  </div>
</template>

<script>
import { onMounted, onUnmounted, ref } from 'vue';

export default {
  name: 'MyComponent',
  setup() {
    let canvas = ref(null);
    let background = ref(null);
    let foreground1 = ref(null);
    let foreground2 = ref(null);
    let config = {
      circle: {
        amount: 18,
        layer: 3,
        color: [157, 97, 207],
        alpha: 0.3,
      },
      line: {
        amount: 12,
        layer: 3,
        color: [255, 255, 255],
        alpha: 0.3,
      },
      speed: 0.5,
      angle: 20,
    };

    let M = Math;
    let degree;
    let circles = [];
    let lines = [];
    let wWidth;
    let wHeight;
    let timer;

    let bctx = ref(null);
    let fctx1 = ref(null);
    let fctx2 = ref(null);

    let requestAnimationFrame =
      window.requestAnimationFrame ||
      window.mozRequestAnimationFrame ||
      window.webkitRequestAnimationFrame ||
      window.msRequestAnimationFrame ||
      window.oRequestAnimationFrame ||
      function (callback, element) {
        setTimeout(callback, 1000 / 60);
      };

    let cancelAnimationFrame =
      window.cancelAnimationFrame ||
      window.mozCancelAnimationFrame ||
      window.webkitCancelAnimationFrame ||
      window.msCancelAnimationFrame ||
      window.oCancelAnimationFrame ||
      clearTimeout;

    let setCanvasHeight = () => {
      wWidth = window.innerWidth;
      wHeight = window.innerHeight;

      for (
        let i = 0;
        i < canvas.value.getElementsByTagName('canvas').length;
        i++
      ) {
        canvas.value.getElementsByTagName('canvas')[i].width = wWidth;
        canvas.value.getElementsByTagName('canvas')[i].height = wHeight;
      }
    };

    let drawCircle = (x, y, radius, color, alpha) => {
      let gradient = fctx1.value.createRadialGradient(x, y, radius, x, y, 0);
      gradient.addColorStop(
        0,
        `rgba(${color[0]},${color[1]},${color[2]},${alpha})`,
      );
      gradient.addColorStop(
        1,
        `rgba(${color[0]},${color[1]},${color[2]},${alpha - 0.1})`,
      );

      fctx1.value.beginPath();
      fctx1.value.arc(x, y, radius, 0, M.PI * 2, true);
      fctx1.value.fillStyle = gradient;
      fctx1.value.fill();
    };

    let drawLine = (x, y, width, color, alpha) => {
      let endX = x + M.sin(degree) * width;
      let endY = y - M.cos(degree) * width;
      let gradient = fctx2.value.createLinearGradient(x, y, endX, endY);
      gradient.addColorStop(
        0,
        `rgba(${color[0]},${color[1]},${color[2]},${alpha})`,
      );
      gradient.addColorStop(
        1,
        `rgba(${color[0]},${color[1]},${color[2]},${alpha - 0.1})`,
      );

      fctx2.value.beginPath();
      fctx2.value.moveTo(x, y);
      fctx2.value.lineTo(endX, endY);
      fctx2.value.lineWidth = 3;
      fctx2.value.lineCap = 'round';
      fctx2.value.strokeStyle = gradient;
      fctx2.value.stroke();
    };

    let drawBack = () => {
      bctx.value.clearRect(0, 0, wWidth, wHeight);

      let gradient = [];

      gradient[0] = bctx.value.createRadialGradient(
        wWidth * 0.3,
        wHeight * 0.1,
        0,
        wWidth * 0.3,
        wHeight * 0.1,
        wWidth * 0.9,
      );
      gradient[0].addColorStop(0, 'rgb(0, 26, 77)');
      gradient[0].addColorStop(1, 'transparent');

      bctx.value.translate(wWidth, 0);
      bctx.value.scale(-1, 1);
      bctx.value.beginPath();
      bctx.value.fillStyle = gradient[0];
      bctx.value.fillRect(0, 0, wWidth, wHeight);

      gradient[1] = bctx.value.createRadialGradient(
        wWidth * 0.1,
        wHeight * 0.1,
        0,
        wWidth * 0.3,
        wHeight * 0.1,
        wWidth,
      );
      gradient[1].addColorStop(0, 'rgb(0, 150, 240)');
      gradient[1].addColorStop(0.8, 'transparent');

      bctx.value.translate(wWidth, 0);
      bctx.value.scale(-1, 1);
      bctx.value.beginPath();
      bctx.value.fillStyle = gradient[1];
      bctx.value.fillRect(0, 0, wWidth, wHeight);

      gradient[2] = bctx.value.createRadialGradient(
        wWidth * 0.1,
        wHeight * 0.5,
        0,
        wWidth * 0.1,
        wHeight * 0.5,
        wWidth * 0.5,
      );
      gradient[2].addColorStop(0, 'rgb(40, 20, 105)');
      gradient[2].addColorStop(1, 'transparent');

      bctx.value.beginPath();
      bctx.value.fillStyle = gradient[2];
      bctx.value.fillRect(0, 0, wWidth, wHeight);
    };

    let animate = () => {
      let sin = M.sin(degree);
      let cos = M.cos(degree);

      if (config.circle.amount > 0 && config.circle.layer > 0) {
        fctx1.value.clearRect(0, 0, wWidth, wHeight);
        for (let i = 0, len = circles.length; i < len; i++) {
          let item = circles[i];
          let x = item.x;
          let y = item.y;
          let radius = item.radius;
          let speed = item.speed;

          if (x > wWidth + radius) {
            x = -radius;
          } else if (x < -radius) {
            x = wWidth + radius;
          } else {
            x += sin * speed;
          }

          if (y > wHeight + radius) {
            y = -radius;
          } else if (y < -radius) {
            y = wHeight + radius;
          } else {
            y -= cos * speed;
          }

          item.x = x;
          item.y = y;
          drawCircle(x, y, radius, item.color, item.alpha);
        }
      }

      if (config.line.amount > 0 && config.line.layer > 0) {
        fctx2.value.clearRect(0, 0, wWidth, wHeight);
        for (let j = 0, len = lines.length; j < len; j++) {
          let item = lines[j];
          let x = item.x;
          let y = item.y;
          let width = item.width;
          let speed = item.speed;

          if (x > wWidth + width * sin) {
            x = -width * sin;
          } else if (x < -width * sin) {
            x = wWidth + width * sin;
          } else {
            x += sin * speed;
          }

          if (y > wHeight + width * cos) {
            y = -width * cos;
          } else if (y < -width * cos) {
            y = wHeight + width * cos;
          } else {
            y -= cos * speed;
          }

          item.x = x;
          item.y = y;
          drawLine(x, y, width, item.color, item.alpha);
        }
      }

      timer = requestAnimationFrame(animate);
    };

    let createItem = () => {
      circles = [];
      lines = [];

      if (config.circle.amount > 0 && config.circle.layer > 0) {
        for (let i = 0; i < config.circle.amount / config.circle.layer; i++) {
          for (let j = 0; j < config.circle.layer; j++) {
            circles.push({
              x: M.random() * wWidth,
              y: M.random() * wHeight,
              radius: M.random() * (20 + j * 5) + (20 + j * 5),
              color: config.circle.color,
              alpha: M.random() * 0.2 + (config.circle.alpha - j * 0.1),
              speed: config.speed * (1 + j * 0.5),
            });
          }
        }
      }

      if (config.line.amount > 0 && config.line.layer > 0) {
        for (let m = 0; m < config.line.amount / config.line.layer; m++) {
          for (let n = 0; n < config.line.layer; n++) {
            lines.push({
              x: M.random() * wWidth,
              y: M.random() * wHeight,
              width: M.random() * (20 + n * 5) + (20 + n * 5),
              color: config.line.color,
              alpha: M.random() * 0.2 + (config.line.alpha - n * 0.1),
              speed: config.speed * (1 + n * 0.5),
            });
          }
        }
      }

      cancelAnimationFrame(timer);
      timer = requestAnimationFrame(animate);
      drawBack();
    };

    onMounted(() => {
      canvas.value = document.getElementById('bg');
      background.value = canvas.value.getElementsByTagName('canvas')[0];
      foreground1.value = canvas.value.getElementsByTagName('canvas')[1];
      foreground2.value = canvas.value.getElementsByTagName('canvas')[2];

      bctx.value = background.value.getContext('2d');
      fctx1.value = foreground1.value.getContext('2d');
      fctx2.value = foreground2.value.getContext('2d');

      degree = (config.angle / 360) * M.PI * 2;

      setCanvasHeight();
      createItem();

      window.addEventListener('resize', () => {
        setCanvasHeight();
        createItem();
      });
    });

    onUnmounted(() => {
      window.removeEventListener('resize', () => {
        setCanvasHeight();
        createItem();
      });
    });

    return {};
  },
};
</script>

<style lang="scss" coped>
#bg {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  z-index: -1;
  background: #2196F3;
  canvas {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
  }
}
</style>
