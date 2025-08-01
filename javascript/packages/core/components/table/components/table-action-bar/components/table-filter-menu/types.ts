import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';

export interface TableFilterMenuProps<T extends TableData = TableData> {
  filterableColumns: FilterableColumn<T>[];
}
