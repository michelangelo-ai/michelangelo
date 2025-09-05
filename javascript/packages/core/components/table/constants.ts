import { DEFAULT_PAGE_SIZE } from './components/table-pagination/constants';

import type { TableState } from './types/table-types';

/**
 * For MultiCell column configurations, the data is joined with
 * this string to enable fuzzy matching
 *
 * @example
 * ```
 * const column: ColumnConfig<T> = {
 *   id: 'name',
 *   accessor: 'name',
 *   items: [
 *     {
 *       id: 'name',
 *       accessor: 'name',
 *     },
 *     {
 *       id: 'age',
 *       accessor: 'age',
 *     },
 *   ],
 * };
 *
 * const row: T = {
 *   name: 'John Doe',
 *   age: 30,
 * };
 *
 * const value = getCellValueForColumn(column, row);
 * // 'John Doe__JOIN__30'
 * ```
 */
export const MULTI_COLUMN_DATA_JOIN_STRING = '__JOIN__';

export const TABLE_STATE_DEFAULTS: TableState = {
  globalFilter: '',
  columnFilters: [],
  pagination: {
    pageIndex: 0,
    pageSize: DEFAULT_PAGE_SIZE,
  },
  sorting: [],
  columnOrder: [],
  columnVisibility: {},
  rowSelection: {},
  rowSelectionEnabled: false,
} as const;

export const TABLE_LOCAL_STORAGE_KEY = 'ma-studio-table-settings';
