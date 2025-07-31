import { TABLE_STATE_DEFAULTS } from '#core/components/table/constants';
import { usePersistedTableState } from './use-persisted-table-state';

import type { ControlledTableState } from '#core/components/table/types/table-types';

/**
 * Primary entry point for adding localStorage persistence to Table components.
 * This hook manages table state with automatic localStorage persistence, falling back to
 * {@link TABLE_STATE_DEFAULTS} when values are not found in localStorage.
 *
 * Use this hook to provide a `state` prop to the Table component for persistent
 * user preferences across browser sessions.
 *
 * @example
 * ```tsx
 * // In your table component
 * const tableState = useLocalStorageTableState({
 *   tableSettingsId: 'user-dashboard-table',
 * });
 *
 * return <Table data={data} columns={columns} state={tableState} />;
 *
 * // State persisted as: 'ma-studio-table-settings.user-dashboard-table.globalFilter'
 * ```
 */
export function useLocalStorageTableState({
  tableSettingsId,
}: {
  tableSettingsId: string;
}): ControlledTableState {
  const [globalFilter, setGlobalFilter] = usePersistedTableState<string>(
    `${tableSettingsId}.globalFilter`,
    TABLE_STATE_DEFAULTS.globalFilter
  );

  return {
    globalFilter,
    setGlobalFilter,
  };
}
