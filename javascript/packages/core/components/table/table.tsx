import React from 'react';
import { getCoreRowModel, getFilteredRowModel, useReactTable } from '@tanstack/react-table';
import { StyledTable } from 'baseui/table-semantic';

import { TableActionBar } from './components/table-action-bar/table-action-bar';
import { transformRows } from './components/table-body/row-transformer';
import { TableBody } from './components/table-body/table-body';
import { TableEmptyState } from './components/table-empty-state/table-empty-state';
import { TableErrorState } from './components/table-error-state/table-error-state';
import { transformHeaders } from './components/table-header/header-transformer';
import { TableHeader } from './components/table-header/table-header';
import { TableNoResultsState } from './components/table-no-results-state/table-no-results-state';
import { useColumnTransformer } from './hooks/use-column-transformer';
import { TableContainer } from './styled-components';
import { applyDefaultProps } from './utils/apply-default-props';
import { getTableViewState } from './utils/get-table-view-state';

import type { TableData } from './types/data-types';
import type { TableProps } from './types/table-types';

export function Table<T extends TableData = TableData>(inputProps: TableProps<T>) {
  const props = applyDefaultProps<T>(inputProps);
  const columns = useColumnTransformer<T>(props.columns);
  const [globalFilter, setGlobalFilter] = React.useState('');

  const table = useReactTable<T>({
    data: props.data,
    columns,
    state: {
      globalFilter,
    },
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    globalFilterFn: 'includesString',
  });

  const viewState = getTableViewState({
    dataLength: props.data.length,
    error: props.error,
    loading: props.loading,
    hasFiltersApplied: globalFilter.length > 0,
    filteredLength: table.getFilteredRowModel().rows.length,
  });

  return (
    <TableContainer>
      <TableActionBar
        globalFilter={globalFilter}
        setGlobalFilter={setGlobalFilter}
        configuration={props.actionBarConfig}
      />

      <StyledTable>
        {viewState === 'loading' && <props.loadingView />}

        {viewState === 'error' && <TableErrorState error={props.error!} />}

        {viewState === 'empty' && <TableEmptyState emptyState={props.emptyState} />}

        {viewState === 'filtered-empty' && (
          <TableNoResultsState clearFilters={() => setGlobalFilter('')} />
        )}

        {viewState !== 'error' && (
          <TableHeader<T>
            headers={transformHeaders<T>(table.getHeaderGroups().flatMap((group) => group.headers))}
          />
        )}

        {viewState === 'ready' && (
          <TableBody<T> rows={transformRows<T>(table.getFilteredRowModel().rows)} />
        )}
      </StyledTable>
    </TableContainer>
  );
}
