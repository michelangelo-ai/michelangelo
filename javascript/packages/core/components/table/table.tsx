import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { StyledTable } from 'baseui/table-semantic';

import { TableActionBar } from './components/table-action-bar/table-action-bar';
import { transformRows } from './components/table-body/row-transformer';
import { TableBody } from './components/table-body/table-body';
import { TableEmptyState } from './components/table-empty-state/table-empty-state';
import { TableErrorState } from './components/table-error-state/table-error-state';
import { TableHeader } from './components/table-header/table-header';
import { TableNoResultsState } from './components/table-no-results-state/table-no-results-state';
import { useColumnTransformer } from './hooks/use-column-transformer';
import { TableSelectionProvider } from './plugins/selection/table-selection-provider';
import { TableContainer } from './styled-components';
import { applyDefaultProps } from './utils/apply-default-props';
import { composeTableState } from './utils/compose-table-state';
import { getTableViewState } from './utils/get-table-view-state';
import { transformColumns } from './utils/transform-columns';

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
    ...(state.setSorting ? { onSortingChange: state.setSorting } : {}),
    ...(state.setColumnOrder ? { onColumnOrderChange: state.setColumnOrder } : {}),
    ...(state.setColumnVisibility ? { onColumnVisibilityChange: state.setColumnVisibility } : {}),
    ...(state.setRowSelection ? { onRowSelectionChange: state.setRowSelection } : {}),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    ...(!props.disableSorting
      ? { getSortedRowModel: getSortedRowModel() }
      : { enableSorting: false }),
    ...(!props.disablePagination ? { getPaginationRowModel: getPaginationRowModel() } : {}),
    enableRowSelection: props.enableRowSelection,
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

  const transformedColumns = transformColumns(table.getAllLeafColumns());

  return (
    <TableSelectionProvider
      value={{
        selectedRows: table.getSelectedRowModel().flatRows.map((row) => row.original),
        selectionEnabled: props.enableRowSelection,
        toggleAllRowsSelected: (selected: boolean) => table.toggleAllRowsSelected(selected),
        getIsAllRowsSelected: () => table.getIsAllRowsSelected(),
        getIsSomeRowsSelected: () => table.getIsSomeRowsSelected(),
      }}
    >
      <TableContainer>
        <TableActionBar
          globalFilter={table.getState().globalFilter as string}
          setGlobalFilter={table.setGlobalFilter}
          columnFilters={table.getState().columnFilters}
          setColumnFilters={table.setColumnFilters}
          columns={columns}
          preFilteredRows={table.getPreFilteredRowModel().rows}
          configuration={props.actionBarConfig}
          filterableColumns={transformedColumns.filter((column) => column.canFilter)}
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
              columns={transformedColumns}
              setColumnOrder={table.setColumnOrder}
              setColumnVisibility={table.setColumnVisibility}
              enableRowSelection={props.enableRowSelection}
              isSelected={table.getIsAllRowsSelected()}
              onToggleSelection={(selected: boolean) => table.toggleAllRowsSelected(selected)}
            />
          )}

          {viewState === 'ready' && (
            <TableBody<T>
              rows={transformRows<T>(table.getRowModel().rows)}
              enableRowSelection={props.enableRowSelection}
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
    </TableSelectionProvider>
  );
}
