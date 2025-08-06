import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';

export type ActiveFilterTagListProps<TData extends TableData = TableData> = {
  filterableColumns: FilterableColumn<TData>[];
  preFilteredRows: Array<{ getValue: (columnId: string) => unknown }>;
};

export type ActiveFilterTagProps<TData extends TableData = TableData> = {
  column: FilterableColumn<TData>;
  preFilteredRows: Array<{ getValue: (columnId: string) => unknown }>;
};
