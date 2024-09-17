import { deepParse } from '@/utils/form';
import { cloneDeep, isEqual, isArray } from 'lodash';

export const parseFormItems = (items, context) => {
  return deepParse(items, context);
};

export const handleLinkages = ({ newVal, oldVal, form, formItems }) => {
  const formValue = cloneDeep(form.value);

  for (const item of formItems) {
    if (item.linkage && !isEqual(newVal[item.name], oldVal[item.name])) {
      item.linkage.forEach((v) => {
        const [name, path, child] = v.target.split('.');

        if (v.value != null) {
          // 有路径视为自增组件
          if (path && path === '*' && isArray(formValue[name])) {
            formValue[name] = formValue[name].map((item) => ({
              ...item,
              [child]: v.value,
            }));
          } else {
            formValue[name] = v.value;
          }
        }
      });

      form.value = formValue;
    }

    if (item.children) {
      handleLinkages({ newVal, oldVal, form, formItems: item.children });
    }
  }
};
