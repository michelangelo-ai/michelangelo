import { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
import { Table } from '#core/components/table/table';
import { adaptTableConfigToTableProps } from '#core/components/views/utils/table-view-adapter';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { DetailViewTablePageProps } from './types';

/**
 * Configuration-driven table page component for detail views
 *
 * Automatically handles data fetching via useStudioQuery and table state persistence.
 *
 * @example
 * ```tsx
 * <TablePage
 *   queryConfig={{
 *     service: 'pipelineRun',
 *     endpoint: 'list',
 *     serviceOptions: {
 *       namespace: projectId,
 *     },
 *   }}
 *   tableConfig={{ columns: PIPELINE_RUN_COLUMNS, disableSearch: true }}
 *   pageId="runs"
 * />
 * ```
 */
export function DetailViewTablePage<T extends object = object>({
  queryConfig,
  tableConfig,
  pageId,
}: DetailViewTablePageProps<T>) {
  const { projectId, phase, entity } = useStudioParams('detail');

  const { data, isLoading, error } = useStudioQuery<Record<`${string}List`, { items: T[] }>>({
    queryName: `List${capitalizeFirstLetter(queryConfig.service)}`,
    serviceOptions: {
      namespace: projectId,
      ...queryConfig.serviceOptions,
    },
    clientOptions: queryConfig.clientOptions,
  });

  const tableState = useLocalStorageTableState({
    tableSettingsId: `${phase}/${entity}/${pageId}`,
    filterSettingsId: `${projectId}/${phase}/${entity}/${pageId}`,
  });

  const tableProps = adaptTableConfigToTableProps<T>(tableConfig, {
    data: data?.[`${queryConfig.service}List`]?.items ?? [],
    loading: isLoading,
    error: error ?? undefined,
  });

  return <Table {...tableProps} state={tableState} />;
}
