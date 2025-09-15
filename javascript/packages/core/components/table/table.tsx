import React from 'react';
import {
  getCoreRowModel,
  getExpandedRowModel,
  getFilteredRowModel,
  getGroupedRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { useStyletron } from 'baseui';

import { useScrollRatio } from '#core/hooks/use-scroll';
import { FilterMode } from './components/filter/types';
import { TableActionBar } from './components/table-action-bar/table-action-bar';
import { transformRows } from './components/table-body/row-transformer';
import { TableEmptyState } from './components/table-empty-state/table-empty-state';
import { TableErrorState } from './components/table-error-state/table-error-state';
import { TableNoResultsState } from './components/table-no-results-state/table-no-results-state';
import { useColumnTransformer } from './hooks/use-column-transformer';
import { usePageResetHandler } from './hooks/use-page-reset-handler';
import { TableSelectionProvider } from './plugins/selection/table-selection-provider';
import { useRowSelectionState } from './plugins/selection/use-row-selection-state';
import { StyledTable } from './styled-components';
import { applyDefaultProps } from './utils/apply-default-props';
import { composeTableState } from './utils/compose-table-state';
import { getTableViewState } from './utils/get-table-view-state';
import { globalFilterFn } from './utils/global-filter-fn';
import { transformColumns } from './utils/transform-columns';

import type { TableData } from './types/data-types';
import type { TableProps } from './types/table-types';

export function Table<T extends TableData = TableData>(inputProps: TableProps<T>) {
  const props = applyDefaultProps<T>(inputProps);
  const columns = useColumnTransformer<T>(props.columns);
  const [css, theme] = useStyletron();

  const { state, initialState } = composeTableState(props.state ?? {});
  const { enableRowSelection, setRowSelectionEnabled } = useRowSelectionState({
    state,
    initialState,
  });

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
    ...(initialState.grouping?.length || state.grouping?.length
      ? {
          getGroupedRowModel: getGroupedRowModel(),
          getExpandedRowModel: getExpandedRowModel(),
        }
      : {}),
    ...(props.subRow
      ? {
          getExpandedRowModel: getExpandedRowModel(),
          getRowCanExpand: () => true,
        }
      : {}),
    enableRowSelection,
    // Tanstack/table filters searchable columns for global filter based on the
    // typeof accessed data. In combination with our custom globalFilterFn, we
    // can safely assume all columns are searchable.
    getColumnCanGlobalFilter: () => true,
    globalFilterFn: globalFilterFn<T>,
    autoResetPageIndex: false,
  });

  const transformedColumns = transformColumns(table.getAllLeafColumns());

  const { isResetting } = usePageResetHandler({
    gotoPage: table.setPageIndex,
    pageCount: table.getPageCount(),
    paginationState: table.getState().pagination,
  });

  const viewState = getTableViewState({
    dataLength: props.data.length,
    error: props.error,
    loading: props.loading || isResetting,
    hasFiltersApplied:
      (table.getState().globalFilter as string)?.length > 0 ||
      (table.getState().columnFilters?.length ?? 0) > 0,
    filteredLength: table.getRowModel().rows.length,
    columnsLength: columns.length,
  });

  // Create lightweight row objects for filter components that only need getValue()
  const preFilteredRows = props.unFilteredData.map((rowData) => ({
    getValue: (columnId: string) => {
      const column = columns.find((col) => col.id === columnId);
      if (!column) return undefined;
      return column.accessorFn(rowData);
    },
    record: rowData,
  }));

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale400 })}>
      <TableSelectionProvider
        value={{
          selectedRows: transformRows<T>(table.getSelectedRowModel().flatRows),
          selectionEnabled: enableRowSelection,
          setSelectionEnabled: setRowSelectionEnabled,
          toggleAllRowsSelected: (selected: boolean) => table.toggleAllRowsSelected(selected),
          getIsAllRowsSelected: () => table.getIsAllRowsSelected(),
          getIsSomeRowsSelected: () => table.getIsSomeRowsSelected(),
        }}
      >
        <TableActionBar<T>
          globalFilter={table.getState().globalFilter as string}
          setGlobalFilter={table.setGlobalFilter}
          columnFilters={table.getState().columnFilters}
          setColumnFilters={table.setColumnFilters}
          preFilteredRows={preFilteredRows}
          configuration={props.actionBarConfig}
          filterableColumns={transformedColumns.filter(
            (column) => column.canFilter && column.filterMode !== FilterMode.NONE
          )}
        />

        <div
          className={css({ overflow: 'auto', position: 'relative' })}
          ref={tableRef as React.RefObject<HTMLDivElement>}
          onScroll={updateScrollRatio}
        >
          <StyledTable>
            {viewState === 'loading' && <props.loadingView />}

            {viewState === 'error' && <TableErrorState error={props.error!} />}

            {(viewState === 'empty' || viewState === 'no-columns') && (
              <TableEmptyState emptyState={props.emptyState} />
            )}

            {viewState === 'filtered-empty' && (
              <TableNoResultsState
                clearFilters={() => {
                  table.setGlobalFilter('');
                  table.setColumnFilters([]);
                }}
              />
            )}

            {viewState !== 'error' && viewState !== 'no-columns' && (
              <props.header
                columns={transformedColumns}
                setColumnOrder={table.setColumnOrder}
                setColumnVisibility={table.setColumnVisibility}
                enableRowSelection={enableRowSelection}
                isSelected={table.getIsAllRowsSelected()}
                onToggleSelection={(selected: boolean) => table.toggleAllRowsSelected(selected)}
                enableStickySides={props.enableStickySides}
                scrollRatio={scrollRatio}
              />
            )}

            {viewState === 'ready' && (
              <props.body
                rows={transformRows<T>(table.getRowModel().rows)}
                enableRowSelection={enableRowSelection}
                enableStickySides={props.enableStickySides}
                scrollRatio={scrollRatio}
                subRow={props.subRow}
                actions={props.actions}
              />
            )}
          </StyledTable>
        </div>
        {!props.disablePagination &&
          viewState === 'ready' &&
          table.getPreFilteredRowModel().rows.length >
            Math.min(...props.pageSizes.map((size) => size.id)) && (
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
