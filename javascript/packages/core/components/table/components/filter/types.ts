import type { Row } from '@tanstack/react-table';
import type { TableData } from '../../types/data-types';

/**
 * Filter function type that matches TanStack Table's filterfn interface.
 * Includes the autoRemove property for automatic filter value removal.
 */
export type TableFilterFn<TData extends TableData = TableData, FilterValueT = unknown> = {
  (row: Row<TData>, columnId: string, filterValue: FilterValueT): boolean;
  autoRemove?: (filterValue: FilterValueT) => boolean;
};

/**
 * Core filter interface that defines the contract for all filter implementations.
 */
export interface FilterHook<TData extends TableData = TableData, FilterValueT = unknown> {
  /**
   * Determines if the provided filterValue indicates that the column is actively being
   * filtered.
   */
  isFilterInactive(filterValue: FilterValueT): boolean;

  /**
   * Builds a user-friendly string for displaying the current filters.
   */
  getActiveFilter(filterValue: FilterValueT): string;

  /**
   * Builds a user-friendly string for summarizing the current filters.
   */
  getFilterSummary(filterValue: FilterValueT): string;

  /**
   * Builds a filter function that can be provided to a tanstack/table column
   * definition.
   */
  buildTableFilterFn(): TableFilterFn<TData, FilterValueT>;
}

/**
 * Enum representing the different modes of filtering that can be applied.
 */
export enum FilterMode {
  NONE = 'none',
  CLIENT = 'client',
  SERVER = 'server',
}

/**
 * Column filter props for filter components.
 */
export type ColumnFilterProps = {
  columnId: string;
  close: () => void;
  getFilterValue: () => unknown;
  setFilterValue: (value: unknown) => void;
  preFilteredRows: Array<{ getValue: (columnId: string) => unknown }>;
};
