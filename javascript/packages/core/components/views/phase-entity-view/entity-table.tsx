import { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
import { Table } from '#core/components/table/table';
import { adaptTableConfigToTableProps } from '#core/components/views/utils/table-view-adapter';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { EntityTableProps } from './types';

/**
 * Generic table component that renders entity data using configuration-driven queries.
 *
 * @example
 * ```tsx
 * // Renders pipelines table with query 'ListPipeline' and data from 'pipelineList.items'
 * <EntityTable
 *   service="pipeline"
 *   tableConfig={{ columns: PIPELINE_COLUMNS, disableSearch: true }}
 *   tableSettingsId="train-pipelines"
 * />
 * ```
 */
export function EntityTable<T extends object = object>({
  service,
  tableConfig,
  tableSettingsId,
}: EntityTableProps<T>) {
  const { projectId } = useStudioParams('base');

  const { data, isLoading, error } = useStudioQuery<Record<`${string}List`, { items: T[] }>>({
    queryName: `List${capitalizeFirstLetter(service)}`,
    serviceOptions: {
      namespace: projectId,
    },
  });

  const entityTableState = useLocalStorageTableState({
    filterSettingsId: `${projectId}/${tableSettingsId}`,
    tableSettingsId,
  });

  const tableProps = adaptTableConfigToTableProps<T>(tableConfig, {
    data: data?.[`${service}List`]?.items ?? [],
    loading: isLoading,
    error: error ?? undefined,
  });

  return <Table {...tableProps} state={entityTableState} />;
}
