import { CellType } from '#core/components/cell/constants';

import type { Header } from '@tanstack/react-table';
import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

/**
 * Transforms TanStack headers into FilterableColumn format
 */
export function transformFilterableColumns<T extends TableData = TableData>(
  tanstackHeaders: Header<T, unknown>[]
): FilterableColumn<T>[] {
  return tanstackHeaders
    .filter((header) => header.column.getCanFilter())
    .map((header) => {
      const columnConfig = header.column.columnDef.meta as ColumnConfig<T>;
      const title = columnConfig.label ?? header.id;

      return {
        id: header.id,
        title,
        columnType: columnConfig.type ?? CellType.TEXT,
      };
    });
}
