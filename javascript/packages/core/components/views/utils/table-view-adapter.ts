import type { TableActionBarConfig } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableProps } from '#core/components/table/types/table-types';
import type { ApplicationError } from '#core/types/error-types';
import type { TableConfig } from '../types';

/**
 * Converts TableConfig configuration to TableProps for the core Table component.
 *
 * This adapter bridges the gap between table configuration leveraged by configuration
 * driven views and the specific props required by the Table component.
 */
export function adaptTableConfigToTableProps<T extends TableData = TableData>(
  config: TableConfig<T>,
  runtimeProps: {
    data: Array<T>;
    loading: boolean;
    error?: ApplicationError;
  }
): TableProps<T> {
  const actionBarConfig: TableActionBarConfig = {
    enableSearch: !config.disableSearch,
    enableFilters: !config.disableFilters,
  };

  return {
    data: runtimeProps.data,
    loading: runtimeProps.loading,
    error: runtimeProps.error,
    columns: config.columns,
    emptyState: config.emptyState,
    actionBarConfig,
    disablePagination: config.disablePagination,
    disableSorting: config.disableSorting,
    pageSizes: config.pageSizes,
    enableStickySides: config.enableStickySides,
  };
}
