import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { StyledTable } from 'baseui/table-semantic';

import { transformFilterableColumns } from './components/table-action-bar/components/table-filter-menu/transform-filterable-columns';
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
import { composeTableState } from './utils/compose-table-state';
import { getTableViewState } from './utils/get-table-view-state';

import type { TableData } from './types/data-types';
import type { TableProps } from './types/table-types';

export function Table<T extends TableData = TableData>(inputProps: TableProps<T>) {
  const props = applyDefaultProps<T>(inputProps);
  const columns = useColumnTransformer<T>(props.columns);

  const { state, initialState } = composeTableState(props.state ?? {});

  const table = useReactTable<T>({
    data: props.data,
    columns,
    initialState,
    ...(Object.keys(state).length > 0 && { state }),
    ...(state.setGlobalFilter ? { onGlobalFilterChange: state.setGlobalFilter } : {}),
    ...(state.setColumnFilters ? { onColumnFiltersChange: state.setColumnFilters } : {}),
    ...(state.setPagination ? { onPaginationChange: state.setPagination } : {}),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    ...(!props.disablePagination ? { getPaginationRowModel: getPaginationRowModel() } : {}),
    globalFilterFn: 'includesString',
  });

  const viewState = getTableViewState({
    dataLength: props.data.length,
    error: props.error,
    loading: props.loading,
    hasFiltersApplied:
      (table.getState().globalFilter as string)?.length > 0 ||
      (table.getState().columnFilters?.length ?? 0) > 0,
    filteredLength: table.getFilteredRowModel().rows.length,
  });

  return (
    <TableContainer>
      <TableActionBar
        globalFilter={table.getState().globalFilter as string}
        setGlobalFilter={table.setGlobalFilter}
        columnFilters={table.getState().columnFilters}
        setColumnFilters={table.setColumnFilters}
        columns={columns}
        preFilteredRows={table.getPreFilteredRowModel().rows}
        configuration={props.actionBarConfig}
        filterableColumns={transformFilterableColumns(
          table.getHeaderGroups().flatMap((group) => group.headers)
        )}
      />

      <StyledTable>
        {viewState === 'loading' && <props.loadingView />}

        {viewState === 'error' && <TableErrorState error={props.error!} />}

        {viewState === 'empty' && <TableEmptyState emptyState={props.emptyState} />}

        {viewState === 'filtered-empty' && (
          <TableNoResultsState
            clearFilters={() => {
              table.setGlobalFilter('');
              table.setColumnFilters([]);
            }}
          />
        )}

        {viewState !== 'error' && (
          <TableHeader<T>
            headers={transformHeaders<T>(table.getHeaderGroups().flatMap((group) => group.headers))}
          />
        )}

        {viewState === 'ready' && (
          <TableBody<T>
            rows={transformRows<T>(
              !props.disablePagination
                ? table.getPaginationRowModel().rows
                : table.getFilteredRowModel().rows
            )}
          />
        )}
      </StyledTable>

      {!props.disablePagination && viewState === 'ready' && (
        <props.pagination
          gotoPage={table.setPageIndex}
          pageCount={table.getPageCount()}
          setPageSize={table.setPageSize}
          state={table.getState().pagination}
          pageSizes={props.pageSizes}
          fetchPlugin={props.fetchPlugin}
        />
      )}
    </TableContainer>
  );
}
