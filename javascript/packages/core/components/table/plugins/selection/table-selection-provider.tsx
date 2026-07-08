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
    // cast: TableSelectionContext is fixed at T = TableData at module load; Table<T> renders many
    // different T, so each Provider must downcast its value
    <TableSelectionContext.Provider value={value as TableSelectionContextType}>
      {children}
    </TableSelectionContext.Provider>
  );
}
