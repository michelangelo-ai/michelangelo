import * as React from 'react';

import type { TableSelectionContext as TableSelectionContextType } from './types';

export const TableSelectionContext = React.createContext<TableSelectionContextType>({
  selectedRows: [],
  selectionEnabled: false,
  setSelectionEnabled: () => null,
  toggleAllRowsSelected: () => null,
  getIsAllRowsSelected: () => false,
  getIsSomeRowsSelected: () => false,
});

export const useTableSelectionContext = () => React.useContext(TableSelectionContext);
