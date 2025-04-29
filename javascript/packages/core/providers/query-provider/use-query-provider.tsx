import { useContext } from 'react';

import { QueryContext } from './query-context';

export const useQueryProvider = () => {
  return useContext(QueryContext);
};
