import React from 'react';

import { TableSelectionContext } from './table-selection-context';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableSelectionContext as TableSelectionContextType } from './types';

type TableSelectionProviderProps<T extends TableData = TableData> = {
  children: React.ReactNode;
  value: TableSelectionContextType<T>;
};

export function TableSelectionProvider<T extends TableData = TableData>({
  children,
  value,
}: TableSelectionProviderProps<T>) {
  return (
    // cast: T extends TableData; context is typed without the generic to avoid provider/consumer coupling
    <TableSelectionContext.Provider value={value as TableSelectionContextType}>
      {children}
    </TableSelectionContext.Provider>
  );
}
