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
