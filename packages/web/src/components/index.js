import TablePlus from './TablePlus/index.vue';
import FormPlus from './FormPlus';
import FormPlusDrawer from './FormPlusDrawer.vue';
import ListHeader from './ListHeader.vue';
import SelectPlus from './SelectPlus.vue';
import RadioPlus from './RadioPlus.vue';
import FilterInput from './FilterInput.vue';
import NumberInput from './NumberInput.vue';
import ResourceName from './ResourceName.vue';
import FormItem from './FormPlus/FormItem.vue';
import FormRender from './FormPlus/FormRender.vue';
import FormGroup from './FormPlus/FormGroup.vue';
import ItemGroup from './ItemGroup.vue';
import BackHeader from './BackHeader.vue';
import InfoPreview from './InfoPreview.vue';
import ButtonGroup from './ButtonGroup.vue';
import DetailDrawer from './DetailDrawer.vue';
import SliderPlus from './SliderPlus.vue';
import EchartsPlus from './Echarts-plus.vue';
import TimePicker from './TimePicker.vue';
import TextPlus from './TextPlus.vue';
import CodePre from './CodePre.vue';
import FormList from './FormPlus/FormList.vue';

export default (app) => {
  app.component('TablePlus', TablePlus);
  app.component('FormPlus', FormPlus);
  app.component('FormPlusDrawer', FormPlusDrawer);
  app.component('ListHeader', ListHeader);
  app.component('SelectPlus', SelectPlus);
  app.component('RadioPlus', RadioPlus);
  app.component('FilterInput', FilterInput);
  app.component('NumberInput', NumberInput);
  app.component('ResourceName', ResourceName);
  app.component('FormItem', FormItem);
  app.component('FormRender', FormRender);
  app.component('FormGroup', FormGroup);
  app.component('ItemGroup', ItemGroup);
  app.component('BackHeader', BackHeader);
  app.component('InfoPreview', InfoPreview);
  app.component('ButtonGroup', ButtonGroup);
  app.component('DetailDrawer', DetailDrawer);
  app.component('SliderPlus', SliderPlus);
  app.component('EchartsPlus', EchartsPlus);
  app.component('TimePicker', TimePicker);
  app.component('TextPlus', TextPlus);
  app.component('CodePre', CodePre);
  app.component('FormList', FormList);
};
