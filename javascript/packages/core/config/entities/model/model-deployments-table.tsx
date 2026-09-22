import { CellType } from '#core/components/cell/constants';
import { useLocalStorageTableState } from '#core/components/table/plugins/state-persistence/use-local-storage-table-state';
import { Table } from '#core/components/table/table';
import { adaptTableConfigToTableProps } from '#core/components/views/utils/table-view-adapter';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import {
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
  DEPLOYMENT_TARGET_CELL,
  DEPLOYMENT_TYPE_CELL,
} from '../deployment/shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableConfig } from '#core/components/views/types';
import type { DeploymentRecord } from '../deployment/types';

/**
 * Columns mirror the Deployments list page, minus the redundant Model column. Deployments
 * live under the deploy phase, so the name link targets that phase explicitly rather than
 * the model page's own (train) phase.
 */
const MODEL_DEPLOYMENT_COLUMNS: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Deployment name',
    type: CellType.TEXT,
    url: '/${studio.projectId}/deploy/deployments/${row.metadata.name}',
  },
  DEPLOYMENT_TYPE_CELL,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_TARGET_CELL,
  { id: 'metadata.creationTimestamp.seconds', label: 'Creation time', type: CellType.DATE },
  { id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT },
  DEPLOYMENT_STATE_CELL,
];

const MODEL_DEPLOYMENTS_TABLE_CONFIG: TableConfig<object> = {
  columns: MODEL_DEPLOYMENT_COLUMNS,
  disableSearch: true,
  disableFilters: true,
  disablePagination: true,
  // Roughly eight rows; a model can accumulate many deployments over time.
  maxHeight: '420px',
  emptyState: { title: 'Model is not currently deployed' },
};

/**
 * Lists the Deployments whose rolled-out revision is the given model. A Deployment's
 * `status.currentRevision.name` holds the Model's own name (see FetchModel in
 * go/components/deployment/plugins/common/model.go), and the apiserver exposes that indexed
 * field under the storage column `current_revision_name`, which is what the field selector
 * must reference (proto paths are not mapped; see the `michelangelo.api.index` options in
 * proto/api/v2/deployment.proto).
 */
export function ModelDeploymentsTable({
  modelName,
  isModelLoading,
}: {
  modelName?: string;
  isModelLoading: boolean;
}) {
  const { projectId, phase, entity } = useStudioParams('detail');

  const { data, isLoading, error } = useStudioQuery<{
    deploymentList?: { items?: DeploymentRecord[] };
  }>({
    queryName: 'ListDeployment',
    serviceOptions: {
      listOptions: { fieldSelector: `current_revision_name=${modelName ?? ''}` },
    },
    clientOptions: { enabled: !isModelLoading && Boolean(modelName) },
  });

  const tableState = useLocalStorageTableState({
    tableSettingsId: `${phase}/${entity}/deployments`,
    filterSettingsId: `${projectId}/${phase}/${entity}/deployments`,
  });

  const tableProps = adaptTableConfigToTableProps<object>(MODEL_DEPLOYMENTS_TABLE_CONFIG, {
    data: data?.deploymentList?.items ?? [],
    loading: isLoading || isModelLoading,
    error: error ?? undefined,
  });

  return <Table {...tableProps} state={tableState} />;
}
