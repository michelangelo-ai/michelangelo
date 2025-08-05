import type { ReactNode } from 'react';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { ColumnFilter } from '#core/components/table/types/table-types';

export interface TableActionBarProps<T extends TableData = TableData> {
  globalFilter: string;
  setGlobalFilter: (value: string) => void;
  columnFilters: ColumnFilter[];
  setColumnFilters: (filters: ColumnFilter[]) => void;
  columns: ColumnConfig<T>[];
  preFilteredRows: Array<{ getValue: (columnId: string) => unknown }>;
  configuration: TableActionBarConfig;
  filterableColumns?: FilterableColumn<T>[];
}

/**
 * Configuration options for the Table Action Bar component.
 */
export interface TableActionBarConfig {
  /**
   * Indicates whether search functionality is enabled.
   *
   * @default true
   */
  enableSearch?: boolean;

  /**
   * Indicates whether filter menu functionality is enabled.
   *
   * @default true
   */
  enableFilters?: boolean;

  /**
   * ReactNode to be rendered in the middle section of the action bar.
   */
  middle?: ReactNode;

  /**
   * ReactNode to be rendered in the trailing section of the action bar.
   */
  trailing?: ReactNode;
}

export interface FilterableColumn<_T extends TableData = TableData> {
  id: string;
  title: string;
  columnType: string;
}
