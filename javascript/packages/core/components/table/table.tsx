import { getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { StyledTable } from 'baseui/table-semantic';

import { transformRows } from './components/table-body/row-transformer';
import { TableBody } from './components/table-body/table-body';
import { transformHeaders } from './components/table-header/header-transformer';
import { TableHeader } from './components/table-header/table-header';
import { useColumnTransformer } from './hooks/use-column-transformer';
import { TableContainer } from './styled-components';

import type { TableData } from './types/data-types';
import type { TableProps } from './types/table-types';

export function Table<T extends TableData = TableData>(props: TableProps<T>) {
  const columns = useColumnTransformer(props.columns);

  const table = useReactTable<T>({
    data: props.data,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  return (
    <TableContainer>
      <StyledTable>
        <TableHeader<T>
          headers={transformHeaders<T>(table.getHeaderGroups().flatMap((group) => group.headers))}
        />
        <TableBody<T> rows={transformRows<T>(table.getRowModel().rows)} />
      </StyledTable>
    </TableContainer>
  );
}
