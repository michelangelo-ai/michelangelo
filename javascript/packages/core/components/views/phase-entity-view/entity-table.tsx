import { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
import { Table } from '#core/components/table/table';
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
 *   listViewConfig={{ type: 'list', columns: PIPELINE_COLUMNS }}
 *   tableSettingsId="train-pipelines"
 * />
 * ```
 */
export function EntityTable({ service, listViewConfig, tableSettingsId }: EntityTableProps) {
  const { projectId } = useStudioParams('base');

  const { data, isLoading, error } = useStudioQuery<Record<`${string}List`, { items: unknown[] }>>({
    queryName: `List${capitalizeFirstLetter(service)}`,
    serviceOptions: {
      namespace: projectId,
    },
  });

  const entityTableState = useLocalStorageTableState({
    filterSettingsId: `${projectId}/${tableSettingsId}`,
    tableSettingsId,
  });

  return (
    <Table
      data={data?.[`${service}List`]?.items ?? []}
      enableStickySides={true}
      error={error ?? undefined}
      columns={listViewConfig.columns}
      loading={isLoading}
      pageSizes={[
        { id: 1, label: '1' },
        { id: 2, label: '2' },
      ]}
      state={entityTableState}
    />
  );
}
