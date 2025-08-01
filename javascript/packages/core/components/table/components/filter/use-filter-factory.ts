import { CellType } from '#core/components/cell/constants';
import { useCategoricalFilterFactory } from './categorical/use-categorical-filter-factory';
import { useDatetimeFilterFactory } from './datetime/use-datetime-filter-factory';

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
) => FilterHook<T, unknown> {
  const createCategoricalFilter = useCategoricalFilterFactory<T>();
  const createDatetimeFilter = useDatetimeFilterFactory<T>();

  return (column: ColumnConfig<T>) => {
    // Route to appropriate filter based on column type
    if (column.type === CellType.DATE) {
      return createDatetimeFilter(column);
    }

    return createCategoricalFilter(column);
  };
}
