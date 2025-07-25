import { useMemo } from 'react';

import { normalizeColumnAccessor } from '../utils/normalize-column-accessor';

import type { AccessorFn } from '#core/types/common/studio-types';
import type { TableColumn } from '../types/column-types';
import type { TableData } from '../types/data-types';

/**
 * Transforms table columns by adding table-specific properties for data display
 * within {@link ../table.tsx}.
 *
 * @example
 * ```tsx
 * const columns = [
 *   { id: 'name', label: 'Full Name', accessor: 'user.name' },
 *   { id: 'age', label: 'Age', accessor: 'user.age' }
 * ];
 *
 * const transformedColumns = useColumnTransformer(columns);
 * return <Table columns={transformedColumns} />
 * ```
 */
export function useColumnTransformer<T extends TableData = TableData>(
  columns: TableColumn[]
): Array<
  TableColumn<T> & {
    header?: string;
    accessorFn?: AccessorFn<T>;
  }
> {
  return useMemo(() => {
    return columns.map((column: TableColumn<T>) => {
      return {
        ...column,
        accessorFn: normalizeColumnAccessor(column),
        header: column.label,
      };
    });
  }, [columns]);
}
