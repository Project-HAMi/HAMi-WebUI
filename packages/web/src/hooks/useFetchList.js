import { onMounted, ref } from 'vue';
import { isArray } from 'lodash';

const useFetchList = (req, path = 'list') => {
  const list = ref([]);

  const fetchList = async () => {
    const { [path]: listData } = await req;
    if (isArray(listData)) {
      list.value = listData;
    }
  };

  onMounted(fetchList);

  return list;
};

export default useFetchList;
