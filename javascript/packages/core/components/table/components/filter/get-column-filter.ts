import { CellType } from '#core/components/cell/constants';
import { CategoricalFilter } from './categorical/categorical-filter';
import { DatetimeFilter } from './datetime/datetime-filter';

import type { ComponentType } from 'react';
import type { TableData } from '#core/components/table/types/data-types';
import type { ColumnFilterProps } from './types';

/**
 * Returns the appropriate filter component for a given column type
 */
export function getColumnFilter<T extends TableData = TableData>(
  columnType: string
): ComponentType<ColumnFilterProps<T>> {
  switch (
    // cast: columnType is a plain string; asserting CellType so case labels compare against the
    // enum's known values
    columnType as CellType
  ) {
    case CellType.DATE:
      return DatetimeFilter;
    default:
      return CategoricalFilter;
  }
}
