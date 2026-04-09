import type { ReactNode } from 'react';
import type { FilterableRow } from '#core/components/table/components/filter/types';
import type {
  ColumnConfig,
  ColumnRenderState,
  FilteringCapability,
} from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { ColumnFilter, TableState } from '#core/components/table/types/table-types';

export interface TableActionBarProps<T extends TableData = TableData> {
  globalFilter: string;
  setGlobalFilter: (value: string) => void;
  columnFilters: ColumnFilter[];
  setColumnFilters: (filters: ColumnFilter[]) => void;
  preFilteredRows: FilterableRow<T>[];
  configuration: TableActionBarConfig;
  filterableColumns?: FilterableColumn<T>[];
  /** Function that builds a shareable URL for the current table state. Required when configuration.enableShareUrl is true. */
  getShareUrl?: () => string;
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
   * Indicates whether a "Copy link" button is rendered that copies a shareable
   * URL encoding the current table state to the clipboard.
   * Requires `validColumnIds` and `urlFilters: { enabled: true }` to be configured
   * on the `useLocalStorageTableState` hook.
   *
   * @default false
   */
  enableShareUrl?: boolean;

  /**
   * ReactNode to be rendered in the middle section of the action bar.
   */
  middle?: ReactNode;

  /**
   * ReactNode to be rendered in the trailing section of the action bar.
   */
  trailing?: ReactNode;
}

export type FilterableColumn<TData extends TableData = TableData> = Omit<
  ColumnConfig<TData>,
  keyof ColumnRenderState<TData>
> &
  ColumnRenderState<TData> &
  Omit<FilteringCapability, 'canFilter'>;

// Re-exported for convenience — callers building share URLs directly can import from here.
export type { TableState };
