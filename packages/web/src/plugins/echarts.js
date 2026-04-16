import { LineChart, BarChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from 'echarts/components';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';

export default function installEcharts() {
  use([CanvasRenderer, LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent]);
}
