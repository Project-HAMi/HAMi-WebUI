import { cloneDeep } from 'lodash';

const getFields = (items = [], context) => {
  const newItems = cloneDeep(items);

  return newItems.reduce((all, current) => {
    const { name, props, children } = current;
    const { $form, $selectData } = context;
    if (name) {
      all[name] = current;
      // 先赋原值
      $form[name] && (all[name].value = $form[name]);

      // 如果有选项数据源的，再次根据labelKey赋值
      if ($selectData[name]) {
        all[name].selectData = $selectData[name];

        //默认优先赋值labelKey
        all[name].value =
          (props.formatter
            ? props.formatter($selectData[name][props.labelKey || 'label'])
            : $selectData[name][props.labelKey || 'label']) || $form[name];

        //多选处理
        if (props.multiple) {
          all[name].value = $selectData[name].map(
            (item) => item[props.labelKey],
          );
        }
      }
    }
    if (children) {
      Object.assign(all, getFields(children, context));
    }
    return all;
  }, {});
};

export default getFields;
