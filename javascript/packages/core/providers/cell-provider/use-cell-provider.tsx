import { useContext } from 'react';

import { CellContext } from './cell-context';

export const useCellProvider = () => {
  return useContext(CellContext);
};
