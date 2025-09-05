import type { FilterableRow } from '#core/components/table/components/filter/types';
import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';
import type { ColumnFilter } from '#core/components/table/types/table-types';

export interface TableFilterMenuContentProps<T extends TableData = TableData> {
  filterableColumns: FilterableColumn<T>[];
  selectedColumn?: FilterableColumn<T>;
  setSelectedColumn: (column: FilterableColumn<T> | undefined) => void;
  columnFilters: ColumnFilter[];
  setColumnFilters: (filters: ColumnFilter[]) => void;
  preFilteredRows: FilterableRow<T>[];
  onClose: () => void;
}
