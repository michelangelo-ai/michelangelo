import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';

export interface TableFilterOptionListProps<T extends TableData = TableData> {
  filterableColumns: FilterableColumn<T>[];
  setSelectedColumn: (column: FilterableColumn<T>) => void;
}
