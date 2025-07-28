import { getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { StyledTable } from 'baseui/table-semantic';

import { transformRows } from './components/table-body/row-transformer';
import { TableBody } from './components/table-body/table-body';
import { TableEmptyState } from './components/table-empty-state/table-empty-state';
import { transformHeaders } from './components/table-header/header-transformer';
import { TableHeader } from './components/table-header/table-header';
import { useColumnTransformer } from './hooks/use-column-transformer';
import { TableContainer } from './styled-components';
import { applyDefaultProps } from './utils/apply-default-props';
import { getTableViewState } from './utils/get-table-view-state';

import type { TableData } from './types/data-types';
import type { TableProps } from './types/table-types';

export function Table<T extends TableData = TableData>(inputProps: TableProps<T>) {
  const props = applyDefaultProps<T>(inputProps);
  const columns = useColumnTransformer<T>(props.columns);

  const table = useReactTable<T>({
    data: props.data,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  const viewState = getTableViewState({
    dataLength: props.data.length,
    loading: props.loading,
  });

  return (
    <TableContainer>
      <StyledTable>
        {viewState === 'loading' && <props.loadingView />}

        {viewState === 'empty' && <TableEmptyState emptyState={props.emptyState} />}

        {viewState === 'ready' && (
          <>
            <TableHeader<T>
              headers={transformHeaders<T>(
                table.getHeaderGroups().flatMap((group) => group.headers)
              )}
            />
            <TableBody<T> rows={transformRows<T>(table.getRowModel().rows)} />
          </>
        )}
      </StyledTable>
    </TableContainer>
  );
}
