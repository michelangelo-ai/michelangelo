import { ActionsPopover } from '#core/components/actions/actions-popover';

import type { ActionSchema, Data } from '#core/components/actions/types';
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
    actions:
      'actions' in config && config.actions
        ? ({ row }: { row: { record: T } }) => (
            // Actions require Record<string, unknown> but TableData is `unknown` — cast at the
            // table/actions boundary since entity records are always objects in practice.
            <ActionsPopover
              actions={config.actions as ActionSchema<Data>[]}
              record={row.record as Data}
            />
          )
        : undefined,
    actionBarConfig,
    disablePagination: config.disablePagination,
    disableSorting: config.disableSorting,
    pageSizes: config.pageSizes,
    enableStickySides: config.enableStickySides,
  };
}
