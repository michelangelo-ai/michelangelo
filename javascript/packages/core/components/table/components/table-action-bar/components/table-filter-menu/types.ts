import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';
import type { ColumnFilter } from '#core/components/table/types/table-types';

export interface TableFilterMenuProps<T extends TableData = TableData> {
  filterableColumns: FilterableColumn<T>[];
  columnFilters: ColumnFilter[];
  setColumnFilters: (filters: ColumnFilter[]) => void;
  preFilteredRows: Array<{ getValue: (columnId: string) => unknown }>;
}
