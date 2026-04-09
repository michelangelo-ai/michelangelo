import { useState } from 'react';

import { TABLE_STATE_DEFAULTS } from '#core/components/table/constants';
import { usePersistedTableState } from './use-persisted-table-state';
import { DEFAULT_PARAM_PREFIX, DEFAULT_URL_SCOPE } from './url-codecs';
import { useUrlTableState } from './use-url-table-state';

import type { UrlFiltersConfig, UrlScope } from './url-codecs';
import type {
  ColumnFilter,
  ColumnOrderState,
  ColumnVisibilityState,
  ControlledTableState,
  GroupingState,
  InputTableState,
  PaginationState,
  RowSelectionState,
  SortingState,
  TableState,
} from '#core/components/table/types/table-types';

export type { UrlFiltersConfig, UrlScope };

/**
 * Primary entry point for adding localStorage persistence to Table components.
 * This hook manages table state with automatic localStorage persistence.
 *
 * **State Priority (highest to lowest):**
 * 1. **URL state** from query parameters (when `urlFilters.enabled = true` and params are present)
 * 2. **Persisted state** from localStorage (user's saved preferences)
 * 3. **Initial state** from props (schema defaults, initial configuration)
 * 4. **Table defaults** from {@link TABLE_STATE_DEFAULTS}
 *
 * **Persistence Strategy:**
 * - **Global settings** (columnVisibility, columnOrder, sorting, pageSize): Persist across all projects using tableSettingsId
 * - **Filter settings** (globalFilter, columnFilters): Persist using filterSettingsId if provided, otherwise tableSettingsId
 *
 * **URL state (opt-in):**
 * When `urlFilters.enabled = true`, query parameters take priority over localStorage for the
 * configured scope. Use `buildShareUrl` from the return value to generate a shareable link
 * encoding the current table state.
 *
 * Use this hook to provide a `state` prop to the Table component for persistent
 * user preferences across browser sessions.
 *
 * @param tableSettingsId - Unique identifier for global table settings (cross-project)
 * @param filterSettingsId - Optional unique identifier for filter settings (project-specific).
 *   When omitted, filters persist globally (useful for app-wide tables like project lists).
 * @param initialState - Optional initial state to use when no persisted state exists
 * @param urlFilters - Optional URL sync configuration (disabled by default)
 * @param validColumnIds - Column IDs allowed in URL params (required when urlFilters.enabled = true)
 *
 * @example
 * ```tsx
 * // App-wide table (filters and settings both global)
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'projects-table',
 * });
 *
 * // With URL sharing enabled
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'users-table',
 *   filterSettingsId: `users-table.project-${projectId}`,
 *   validColumnIds: columns.map((c) => c.id),
 *   urlFilters: { enabled: true },
 * });
 *
 * return <Table data={data} columns={columns} state={tableState} actionBarConfig={{ enableShareUrl: true }} />;
 * ```
 */
export function useLocalStorageTableState({
  tableSettingsId,
  filterSettingsId,
  initialState,
  urlFilters,
  validColumnIds = [],
}: {
  tableSettingsId: string;
  filterSettingsId?: string;
  initialState?: InputTableState;
  urlFilters?: UrlFiltersConfig;
  validColumnIds?: string[];
}): ControlledTableState & { buildShareUrl: (state: Partial<TableState>) => string } {
  const filterNamespace = filterSettingsId ?? tableSettingsId;
  const urlEnabled = urlFilters?.enabled ?? false;

  const { urlState, buildShareUrl } = useUrlTableState({
    tableSettingsId,
    validColumnIds,
    scope: urlFilters?.scope ?? DEFAULT_URL_SCOPE,
    paramPrefix: urlFilters?.paramPrefix ?? DEFAULT_PARAM_PREFIX,
  });

  const urlGlobalFilter =
    urlEnabled && urlState?.globalFilter !== undefined ? urlState.globalFilter : null;
  const urlColumnFilters =
    urlEnabled && urlState?.columnFilters !== undefined ? urlState.columnFilters : null;
  const urlSorting =
    urlEnabled && urlState?.sorting !== undefined ? urlState.sorting : null;
  const urlColumnVisibility =
    urlEnabled && urlState?.columnVisibility !== undefined ? urlState.columnVisibility : null;

  const [persistedGlobalFilter, setGlobalFilter] = usePersistedTableState<string>(
    `${filterNamespace}.globalFilter`,
    initialState?.globalFilter ?? TABLE_STATE_DEFAULTS.globalFilter
  );
  // URL state has the highest priority — override persisted value when URL param is present
  const globalFilter = urlGlobalFilter !== null ? urlGlobalFilter : persistedGlobalFilter;

  const [persistedColumnFilters, setColumnFilters] = usePersistedTableState<ColumnFilter[]>(
    `${filterNamespace}.columnFilters`,
    initialState?.columnFilters ?? TABLE_STATE_DEFAULTS.columnFilters
  );
  const columnFilters =
    urlColumnFilters !== null ? urlColumnFilters : persistedColumnFilters;

  const [persistedSorting, setSorting] = usePersistedTableState<SortingState>(
    `${tableSettingsId}.sorting`,
    initialState?.sorting ?? TABLE_STATE_DEFAULTS.sorting
  );
  const sorting = urlSorting !== null ? urlSorting : persistedSorting;

  const [persistedColumnVisibility, setColumnVisibility] =
    usePersistedTableState<ColumnVisibilityState>(
      `${tableSettingsId}.columnVisibility`,
      initialState?.columnVisibility ?? TABLE_STATE_DEFAULTS.columnVisibility
    );
  const columnVisibility =
    urlColumnVisibility !== null ? urlColumnVisibility : persistedColumnVisibility;

  const [pageSize, setPageSize] = usePersistedTableState<number>(
    `${tableSettingsId}.pageSize`,
    initialState?.pagination?.pageSize ?? TABLE_STATE_DEFAULTS.pagination.pageSize
  );

  const [columnOrder, setColumnOrder] = usePersistedTableState<ColumnOrderState>(
    `${tableSettingsId}.columnOrder`,
    initialState?.columnOrder ?? TABLE_STATE_DEFAULTS.columnOrder
  );

  // Not persisted on reload
  const [pageIndex, setPageIndex] = useState<number>(0);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [rowSelectionEnabled, setRowSelectionEnabled] = useState<boolean>(
    initialState?.rowSelectionEnabled ?? TABLE_STATE_DEFAULTS.rowSelectionEnabled
  );
  const [grouping, setGrouping] = useState<GroupingState>(
    initialState?.grouping ?? TABLE_STATE_DEFAULTS.grouping
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
    grouping,
    setGrouping,
    buildShareUrl,
  };
}
