import { CellType } from '#core/components/cell/constants';

import type { Column } from '@tanstack/react-table';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type {
  ColumnRenderState,
  FilteringCapability,
  SortingCapability,
} from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

/**
 * Create a list of columns that can be used when rendering #core/components/table/*
 */
export function transformColumns<T extends TableData = TableData>(
  columns: Column<T, unknown>[]
): Array<ColumnRenderState<T> & FilteringCapability & SortingCapability> {
  return columns.map((column) => {
    const columnConfig = column.columnDef.meta as ColumnConfig<T>;
    const label = columnConfig.label ?? column.id;

    return {
      id: column.id,
      label,
      type: columnConfig.type ?? CellType.TEXT,

      canFilter: column.getCanFilter(),
      getFilterValue: column.getFilterValue,
      setFilterValue: column.setFilterValue,

      canSort: column.getCanSort(),
      onToggleSort: column.getToggleSortingHandler() ?? (() => undefined),
      sortDirection: column.getIsSorted(),
    };
  });
}
