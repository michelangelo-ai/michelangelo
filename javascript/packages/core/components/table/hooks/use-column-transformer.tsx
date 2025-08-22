import { ReactNode, useMemo } from 'react';
import { CellContext } from '@tanstack/react-table';

import { useFilterFactory } from '../components/filter/use-filter-factory';
import { TableCell } from '../components/table-cell/table-cell';
import { normalizeColumnAccessor } from '../utils/normalize-column-accessor';

import type { AccessorFn } from '#core/types/common/studio-types';
import type { TableFilterFn } from '../components/filter/types';
import type { ColumnConfig } from '../types/column-types';
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
  columns: ColumnConfig[]
): {
  id: string;
  header?: string;
  accessorFn: AccessorFn<T>;
  meta: ColumnConfig<T>;
  cell: (props: CellContext<T, unknown>) => ReactNode;
  filterFn?: TableFilterFn<T, unknown[]>;
}[] {
  const createFilter = useFilterFactory<T>();

  return useMemo(() => {
    return columns.map((column: ColumnConfig<T>) => {
      const filterHook = createFilter(column);

      return {
        id: column.id,
        meta: column,
        accessorFn: normalizeColumnAccessor(column),
        header: column.label,
        cell: (props: CellContext<T, unknown>) => (
          <TableCell
            column={props.column.columnDef.meta as ColumnConfig}
            record={props.row.original as object}
            value={props.getValue()}
            columnFilterValue={props.column.getFilterValue()}
            setColumnFilterValue={props.column.setFilterValue}
          />
        ),
        filterFn: filterHook.buildTableFilterFn(),
        enableSorting: column.enableSorting ?? true,
        sortUndefined: 'last',
      };
    });
  }, [columns, createFilter]);
}
