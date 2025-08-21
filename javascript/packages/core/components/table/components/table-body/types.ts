import type { ReactNode } from 'react';
import type { SelectableCapability } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

export type TableCell<_T extends TableData = TableData> = {
  id: string;
  content: ReactNode;
};

export type TableRow<T extends TableData = TableData> = {
  id: string;
  cells: TableCell<T>[];
} & SelectableCapability;

export type TableBodyProps<T extends TableData = TableData> = {
  rows: TableRow<T>[];
  enableRowSelection?: boolean;
};
