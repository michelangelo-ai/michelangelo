import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';

export interface TableFilterMenuContentProps<T extends TableData = TableData> {
  filterableColumns: FilterableColumn<T>[];
  selectedColumn?: FilterableColumn<T>;
  onColumnSelect: (column: FilterableColumn<T>) => void;
}
