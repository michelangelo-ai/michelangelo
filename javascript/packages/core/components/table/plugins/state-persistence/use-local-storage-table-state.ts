import { useState } from 'react';

import { TABLE_STATE_DEFAULTS } from '#core/components/table/constants';
import { usePersistedTableState } from './use-persisted-table-state';

import type {
  ColumnFilter,
  ColumnOrderState,
  ColumnVisibilityState,
  ControlledTableState,
  PaginationState,
  RowSelectionState,
  SortingState,
} from '#core/components/table/types/table-types';

/**
 * Primary entry point for adding localStorage persistence to Table components.
 * This hook manages table state with automatic localStorage persistence.
 *
 * **State Priority (highest to lowest):**
 * 1. **Persisted state** from localStorage (user's saved preferences)
 * 2. **Initial state** from props (schema defaults, initial configuration)
 * 3. **Table defaults** from {@link TABLE_STATE_DEFAULTS}
 *
 * Use this hook to provide a `state` prop to the Table component for persistent
 * user preferences across browser sessions.
 *
 * @param tableSettingsId - Unique identifier for this table's settings in localStorage
 * @param initialState - Optional initial state to use when no persisted state exists
 *
 * @example
 * ```tsx
 * // Basic usage
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'user-dashboard-table',
 * });
 *
 * // With initial state (e.g., hidden columns from schema)
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'user-dashboard-table',
 *   initialState: {
 *     columnVisibility: { hiddenColumnId: false },
 *     sorting: [{ id: 'name', desc: false }],
 *   },
 * });
 *
 * return <Table data={data} columns={columns} state={tableState} />;
 *
 * // State persisted as: 'ma-studio-table-settings.user-dashboard-table.globalFilter'
 * ```
 */
export function useLocalStorageTableState({
  tableSettingsId,
  initialState,
}: {
  tableSettingsId: string;
  initialState?: Partial<ControlledTableState>;
}): ControlledTableState {
  const [globalFilter, setGlobalFilter] = usePersistedTableState<string>(
    `${tableSettingsId}.globalFilter`,
    initialState?.globalFilter ?? TABLE_STATE_DEFAULTS.globalFilter
  );

  const [columnFilters, setColumnFilters] = usePersistedTableState<ColumnFilter[]>(
    `${tableSettingsId}.columnFilters`,
    initialState?.columnFilters ?? TABLE_STATE_DEFAULTS.columnFilters
  );

  const [pageSize, setPageSize] = usePersistedTableState<number>(
    `${tableSettingsId}.pageSize`,
    initialState?.pagination?.pageSize ?? TABLE_STATE_DEFAULTS.pagination.pageSize
  );

  const [sorting, setSorting] = usePersistedTableState<SortingState>(
    `${tableSettingsId}.sorting`,
    initialState?.sorting ?? TABLE_STATE_DEFAULTS.sorting
  );

  const [columnOrder, setColumnOrder] = usePersistedTableState<ColumnOrderState>(
    `${tableSettingsId}.columnOrder`,
    initialState?.columnOrder ?? TABLE_STATE_DEFAULTS.columnOrder
  );

  const [columnVisibility, setColumnVisibility] = usePersistedTableState<ColumnVisibilityState>(
    `${tableSettingsId}.columnVisibility`,
    initialState?.columnVisibility ?? TABLE_STATE_DEFAULTS.columnVisibility
  );

  // pageIndex and rowSelection are not persisted (reset on reload)
  const [pageIndex, setPageIndex] = useState<number>(0);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});

  return {
    globalFilter,
    setGlobalFilter,
    columnFilters,
    setColumnFilters,
    pagination: {
      pageIndex,
      pageSize,
    },
    setPagination: (updater: PaginationState | ((old: PaginationState) => PaginationState)) => {
      const currentState = { pageIndex, pageSize };
      const newState = typeof updater === 'function' ? updater(currentState) : updater;
      setPageIndex(newState.pageIndex);
      setPageSize(newState.pageSize);
    },
    sorting,
    setSorting,
    columnOrder,
    setColumnOrder,
    columnVisibility,
    setColumnVisibility,
    rowSelection,
    setRowSelection,
  };
}
