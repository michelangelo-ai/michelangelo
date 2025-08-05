import { CellType } from '#core/components/cell/constants';
import { CategoricalFilter } from './categorical/categorical-filter';
import { DatetimeFilter } from './datetime/datetime-filter';

import type { ComponentType } from 'react';
import type { ColumnFilterProps } from './types';

/**
 * Returns the appropriate filter component for a given column type
 */
export function getColumnFilter(columnType: string): ComponentType<ColumnFilterProps> {
  switch (columnType as CellType) {
    case CellType.DATE:
      return DatetimeFilter;
    default:
      return CategoricalFilter;
  }
}
