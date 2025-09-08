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
 * **Persistence Strategy:**
 * - **Global settings** (columnVisibility, columnOrder, sorting, pageSize): Persist across all projects using tableSettingsId
 * - **Filter settings** (globalFilter, columnFilters): Persist using filterSettingsId if provided, otherwise tableSettingsId
 *
 * Use this hook to provide a `state` prop to the Table component for persistent
 * user preferences across browser sessions.
 *
 * @param tableSettingsId - Unique identifier for global table settings (cross-project)
 * @param filterSettingsId - Optional unique identifier for filter settings (project-specific).
 *   When omitted, filters persist globally (useful for app-wide tables like project lists).
 * @param initialState - Optional initial state to use when no persisted state exists
 *
 * @example
 * ```tsx
 * // App-wide table (filters and settings both global)
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'projects-table',
 * });
 *
 * // Project-specific filters (recommended for project-scoped data)
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'user-dashboard-table',
 *   filterSettingsId: `user-dashboard-table.project-${projectId}`,
 * });
 *
 * // With initial state (e.g., hidden columns from schema)
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'user-dashboard-table',
 *   filterSettingsId: `user-dashboard-table.project-${projectId}`,
 *   initialState: {
 *     columnVisibility: { hiddenColumnId: false },
 *     sorting: [{ id: 'name', desc: false }],
 *   },
 * });
 *
 * return <Table data={data} columns={columns} state={tableState} />;
 *
 * // State persisted as: 'ma-studio-table-settings.user-dashboard-table.project-${projectId}.globalFilter'
 * ```
 */
export function useLocalStorageTableState({
  tableSettingsId,
  filterSettingsId,
  initialState,
}: {
  tableSettingsId: string;
  filterSettingsId?: string;
  initialState?: Partial<ControlledTableState>;
}): ControlledTableState {
  // Use filterSettingsId for filters when provided, otherwise fall back to tableSettingsId
  const filterNamespace = filterSettingsId ?? tableSettingsId;

  const [globalFilter, setGlobalFilter] = usePersistedTableState<string>(
    `${filterNamespace}.globalFilter`,
    initialState?.globalFilter ?? TABLE_STATE_DEFAULTS.globalFilter
  );

  const [columnFilters, setColumnFilters] = usePersistedTableState<ColumnFilter[]>(
    `${filterNamespace}.columnFilters`,
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
  const [rowSelectionEnabled, setRowSelectionEnabled] = useState<boolean>(
    initialState?.rowSelectionEnabled ?? TABLE_STATE_DEFAULTS.rowSelectionEnabled
  );

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
    rowSelectionEnabled,
    setRowSelectionEnabled,
  };
}
