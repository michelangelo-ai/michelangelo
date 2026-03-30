import { CellType } from '#core/components/cell/constants';
import { PIPELINE_STATE_CELL, PIPELINE_TYPE_CELL } from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const PIPELINE_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    url: '/${studio.projectId}/${studio.phase}/pipelines/${data.metadata.name}',
  },
  { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
  PIPELINE_TYPE_CELL,
  {
    id: 'spec.commit.branch',
    label: 'Branch',
    type: CellType.TEXT,
  },
  PIPELINE_STATE_CELL,
];

export const PIPELINE_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: PIPELINE_CELL_CONFIG,
  },
};
