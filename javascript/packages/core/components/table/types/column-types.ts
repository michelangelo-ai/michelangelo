import '@tanstack/react-table'; //or vue, svelte, solid, qwik, etc.

import type { Cell } from '#core/components/cell/types';
import type { FilterMode } from '../components/filter/types';
import type { TableData } from './data-types';

declare module '@tanstack/react-table' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-object-type
  interface ColumnMeta<TData extends TableData, TValue> extends ColumnConfig<TData> {}
}

export type ColumnConfig<TData extends TableData = TableData> = Cell<TData> & {
  /**
   * @description
   * Configures the filtering mode for the column. If using `FilterMode.SERVER`, ensure
   * that filtering the column generates a valid LIST query
   *
   * @default FilterMode.NONE
   */
  filterMode?: FilterMode;

  /**
   * @default true
   */
  enableSorting?: boolean;
};

/**
 * Base column properties extracted from ColumnConfig for rendering.
 * Provides the minimal column identity needed by table components.
 */
export type ColumnRenderState<TData extends TableData = TableData> = Required<
  Pick<ColumnConfig<TData>, 'id' | 'label' | 'type'>
>;

/**
 * Defines a column's filtering capabilities and current state.
 * Used by filter components to determine available interactions.
 *
 * @example
 * ```ts
 * setFilter('test')
 * expect(getFilterValue()).toBe('test')
 * ```
 */
export type FilteringCapability = {
  canFilter: boolean;
  getFilterValue: () => unknown;
  setFilterValue: (value: unknown) => void;
};

/**
 * Defines a column's sorting capabilities and current state.
 * Used by header components to enable sort interactions.
 *
 * @example
 * ```ts
 * // sortDirection = false
 * onToggleSort(e)
 * expect(sortDirection).toBe('asc')
 * ```
 */
export type SortingCapability = {
  canSort: boolean;
  onToggleSort: (e: React.MouseEvent<HTMLDivElement>) => void;
  sortDirection: false | 'asc' | 'desc';
};
