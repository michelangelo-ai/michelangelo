import { useState } from 'react';

import { TABLE_STATE_DEFAULTS } from '#core/components/table/constants';
import { DEFAULT_PARAM_PREFIX, DEFAULT_URL_SCOPE } from './url-codecs';
import { useLocalStorageTableState } from './use-local-storage-table-state';
import { useUrlTableState } from './use-url-table-state';

import type {
  ColumnFilter,
  ColumnVisibilityState,
  ControlledTableState,
  InputTableState,
  SortingState,
  TableState,
} from '#core/components/table/types/table-types';
import type { UrlFiltersConfig, UrlScope } from './types';

export type { UrlFiltersConfig, UrlScope };

/**
 * Primary hook for table state management with optional URL-based filter sharing.
 *
 * Composes localStorage persistence ({@link useLocalStorageTableState}) with URL-driven
 * filter state ({@link useUrlTableState}). When URL params matching this table are present
 * and `urlFilters.enabled = true`, filter state is seeded from the URL into local in-memory
 * state — users can interact with filters freely, but changes are not written to localStorage.
 * When no matching URL params are present, state falls back to localStorage as normal.
 *
 * Use `buildShareUrl` from the return value to generate a shareable link encoding the
 * current table state.
 *
 * **State source (when urlFilters.enabled = true):**
 * - URL params present → seeded from URL into in-memory state; interactive, not persisted
 * - No URL params → read from localStorage, setters persist to localStorage
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
 * // App-wide table with localStorage only
 * const tableState = useTableState({
 *   tableSettingsId: 'projects-table',
 * });
 *
 * // Project-specific filters with URL sharing enabled
 * const tableState = useTableState({
 *   tableSettingsId: 'users-table',
 *   filterSettingsId: `users-table.project-${projectId}`,
 *   validColumnIds: columns.map((c) => c.id),
 *   urlFilters: { enabled: true },
 * });
 *
 * return <Table data={data} columns={columns} state={tableState} actionBarConfig={{ enableShareUrl: true }} />;
 * ```
 */
export function useTableState({
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
  const { urlState, hasUrlState, buildShareUrl } = useUrlTableState({
    tableSettingsId,
    validColumnIds,
    scope: urlFilters?.scope ?? DEFAULT_URL_SCOPE,
    paramPrefix: urlFilters?.paramPrefix ?? DEFAULT_PARAM_PREFIX,
  });

  const localState = useLocalStorageTableState({
    tableSettingsId,
    filterSettingsId,
    initialState,
  });

  // When URL params are present and urlFilters is enabled, seed in-memory state from the URL.
  // This lets users interact with filters freely without writing to localStorage.
  const urlActive = (urlFilters?.enabled ?? false) && hasUrlState;

  const [memGlobalFilter, setMemGlobalFilter] = useState<string>(
    urlActive
      ? (urlState?.globalFilter ?? TABLE_STATE_DEFAULTS.globalFilter)
      : localState.globalFilter
  );
  const [memColumnFilters, setMemColumnFilters] = useState<ColumnFilter[]>(
    urlActive
      ? (urlState?.columnFilters ?? TABLE_STATE_DEFAULTS.columnFilters)
      : localState.columnFilters
  );
  const [memSorting, setMemSorting] = useState<SortingState>(
    urlActive ? (urlState?.sorting ?? TABLE_STATE_DEFAULTS.sorting) : localState.sorting
  );
  const [memColumnVisibility, setMemColumnVisibility] = useState<ColumnVisibilityState>(
    urlActive
      ? (urlState?.columnVisibility ?? TABLE_STATE_DEFAULTS.columnVisibility)
      : localState.columnVisibility
  );

  return {
    ...localState,
    globalFilter: urlActive ? memGlobalFilter : localState.globalFilter,
    setGlobalFilter: urlActive ? setMemGlobalFilter : localState.setGlobalFilter,
    columnFilters: urlActive ? memColumnFilters : localState.columnFilters,
    setColumnFilters: urlActive ? setMemColumnFilters : localState.setColumnFilters,
    sorting: urlActive ? memSorting : localState.sorting,
    setSorting: urlActive ? setMemSorting : localState.setSorting,
    columnVisibility: urlActive ? memColumnVisibility : localState.columnVisibility,
    setColumnVisibility: urlActive ? setMemColumnVisibility : localState.setColumnVisibility,
    buildShareUrl,
  };
}
