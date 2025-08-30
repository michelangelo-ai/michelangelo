import {
  getCoreRowModel,
  getExpandedRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { useStyletron } from 'baseui';
import { StyledTable } from 'baseui/table-semantic';

import { useScrollRatio } from '#core/hooks/use-scroll';
import { TableActionBar } from './components/table-action-bar/table-action-bar';
import { transformRows } from './components/table-body/row-transformer';
import { TableEmptyState } from './components/table-empty-state/table-empty-state';
import { TableErrorState } from './components/table-error-state/table-error-state';
import { TableHeader } from './components/table-header/table-header';
import { TableNoResultsState } from './components/table-no-results-state/table-no-results-state';
import { useColumnTransformer } from './hooks/use-column-transformer';
import { TableSelectionProvider } from './plugins/selection/table-selection-provider';
import { applyDefaultProps } from './utils/apply-default-props';
import { composeTableState } from './utils/compose-table-state';
import { getTableViewState } from './utils/get-table-view-state';
import { normalizeColumnAccessor } from './utils/normalize-column-accessor';
import { transformColumns } from './utils/transform-columns';

import type { TableData } from './types/data-types';
import type { TableProps } from './types/table-types';

export function Table<T extends TableData = TableData>(inputProps: TableProps<T>) {
  const props = applyDefaultProps<T>(inputProps);
  const columns = useColumnTransformer<T>(props.columns);
  const [css, theme] = useStyletron();

  const { state, initialState } = composeTableState(props.state ?? {});
  const { scrollRatio, tableRef, updateScrollRatio } = useScrollRatio(columns);

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
    ...(props.subRow
      ? {
          getExpandedRowModel: getExpandedRowModel(),
          getRowCanExpand: () => true,
        }
      : {}),
    enableRowSelection: props.enableRowSelection,
    globalFilterFn: 'includesString',
    autoResetPageIndex: false,
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

  // Create lightweight row objects for filter components that only need getValue()
  const preFilteredRows = props.unFilteredData.map((rowData) => ({
    getValue: (columnId: string) => {
      const column = columns.find((col) => col.id === columnId);
      if (!column) return undefined;
      return normalizeColumnAccessor(column)(rowData);
    },
  }));

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale400 })}>
      <TableSelectionProvider
        value={{
          selectedRows: table.getSelectedRowModel().flatRows.map((row) => row.original),
          selectionEnabled: props.enableRowSelection,
          toggleAllRowsSelected: (selected: boolean) => table.toggleAllRowsSelected(selected),
          getIsAllRowsSelected: () => table.getIsAllRowsSelected(),
          getIsSomeRowsSelected: () => table.getIsSomeRowsSelected(),
        }}
      >
        <TableActionBar
          globalFilter={table.getState().globalFilter as string}
          setGlobalFilter={table.setGlobalFilter}
          columnFilters={table.getState().columnFilters}
          setColumnFilters={table.setColumnFilters}
          columns={columns}
          preFilteredRows={preFilteredRows}
          configuration={props.actionBarConfig}
          filterableColumns={transformedColumns.filter((column) => column.canFilter)}
        />

        <div
          className={css({ overflow: 'auto', position: 'relative' })}
          ref={tableRef as React.RefObject<HTMLDivElement>}
          onScroll={updateScrollRatio}
        >
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
                enableStickySides={props.enableStickySides}
                scrollRatio={scrollRatio}
              />
            )}

            {viewState === 'ready' && (
              <props.body
                rows={transformRows<T>(table.getRowModel().rows)}
                enableRowSelection={props.enableRowSelection}
                enableStickySides={props.enableStickySides}
                scrollRatio={scrollRatio}
                subRow={props.subRow}
                actions={props.actions}
              />
            )}
          </StyledTable>
        </div>
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
      </TableSelectionProvider>
    </div>
  );
}
