import '@tanstack/react-table'; //or vue, svelte, solid, qwik, etc.

import type { AggregationFnOption, SortingFnOption } from '@tanstack/react-table';
import type { ComponentType, ReactNode } from 'react';
import type { Cell, CellRendererProps, CellTooltip } from '#core/components/cell/types';
import type { DistributiveOmit } from '#core/types/utility-types';
import type { FilterMode } from '../components/filter/types';
import type { TableData } from './data-types';
import type { TableRow } from './row-types';

declare module '@tanstack/react-table' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-object-type
  interface ColumnMeta<TData extends TableData, TValue> extends ColumnConfig<TData> {}
}

export type ColumnConfig<TData = TableData> = DistributiveOmit<Cell<TData>, 'tooltip'> & {
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

  /**
   * @description
   * Enables grouping functionality for this column. When true, this column can be used
   * as a grouping column and will show expand/collapse controls when grouped.
   *
   * @default false
   */
  enableGrouping?: boolean;

  /**
   * @description
   * Defines how values should be aggregated when this column is grouped.
   * Can be a string identifier for built-in aggregation functions or a custom function.
   *
   * Built-in aggregation functions include: 'count', 'sum', 'min', 'max', 'extent', 'mean', 'median', 'unique', 'uniqueCount'
   *
   * @example
   * ```tsx
   * // Built-in aggregation
   * { aggregationFn: 'count' }
   *
   * // Custom aggregation
   * {
   *   aggregationFn: (columnId, leafRows) => {
   *     return leafRows.reduce((sum, row) => sum + row.getValue(columnId), 0);
   *   }
   * }
   * ```
   *
   * @see https://tanstack.com/table/latest/docs/api/features/grouping#aggregation-functions
   * @default undefined
   */
  aggregationFn?: AggregationFnOption<TData>;

  /**
   * @description
   * Custom cell renderer to use when this column's cells are aggregated in a grouped row.
   * If not provided, the regular Cell renderer will be used.
   *
   * @default undefined
   */
  aggregatedCell?: ComponentType<CellRendererProps<TData, ColumnConfig<TData>>>;

  /**
   * @description Custom sorting function to be applied to this column
   *
   * @default undefined
   * @see https://tanstack.com/table/latest/docs/api/features/sorting#sorting-functions
   */
  sortingFn?: SortingFnOption<TData>;

  /**
   * @description
   * Custom tooltip to be displayed when this column's cells are hovered.
   *
   * @default undefined
   */
  tooltip?: ColumnTooltip<TData>;
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

/**
 * Defines a column's visibility capabilities and current state.
 * Used to hide columns from the table.
 */
export type VisibilityCapability = {
  canHide: boolean;
  isVisible: boolean;
};

/**
 * Defines a column's selection capabilities and current state.
 * Used by selectable cell components to enable row selection interactions.
 *
 * @example
 * ```ts
 * // isSelected = false
 * onToggleSelection(true)
 * expect(isSelected).toBe(true)
 * ```
 */
export type SelectableCapability = {
  canSelect: boolean;
  isSelected: boolean;
  onToggleSelection: (selected: boolean) => void;
};

export type ColumnTooltip<TData extends TableData = TableData> = Omit<CellTooltip, 'content'> & {
  /**
   * @description
   * The content to be displayed in the tooltip.
   *
   * @remarks
   * If a function is provided, it will be called with the cell renderer props and the row data.
   */
  content:
    | string
    | ((
        params: CellRendererProps<TData, ColumnConfig<TData>> & { row: TableRow<TData> }
      ) => ReactNode);
};
