import { CellType } from '#core/components/cell/constants';
import {
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
  DEPLOYMENT_TARGET_CELL,
  DEPLOYMENT_TYPE_CELL,
} from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

const DEPLOYMENT_COLUMNS: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    type: CellType.TEXT,
    url: '/${studio.projectId}/${studio.phase}/deployments/${row.metadata.name}',
  },
  {
    id: 'status.currentRevision.name',
    label: 'Model',
    type: CellType.TEXT,
  },
  DEPLOYMENT_TYPE_CELL,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_TARGET_CELL,
  {
    id: 'spec.owner.name',
    label: 'Owner',
    type: CellType.TEXT,
  },
  DEPLOYMENT_STATE_CELL,
];

export const DEPLOYMENT_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: DEPLOYMENT_COLUMNS,
  },
};
