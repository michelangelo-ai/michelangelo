import { useCategoricalFilterFactory } from './categorical/use-categorical-filter-factory';

import type { ColumnConfig } from '../../types/column-types';
import type { TableData } from '../../types/data-types';
import type { FilterHook } from './types';

/**
 * Hook that returns a factory function for creating filter instances based on column configuration.
 *
 * @returns A factory function that takes a column and returns the appropriate filter instance
 */
export function useFilterFactory<T extends TableData = TableData>(): (
  column: ColumnConfig<T>
) => FilterHook<T, unknown[]> {
  const createCategoricalFilter = useCategoricalFilterFactory<T>();

  return (column: ColumnConfig<T>) => {
    // For now, all filters are categorical. We'll add datetime filter support later.
    return createCategoricalFilter(column);
  };
}
