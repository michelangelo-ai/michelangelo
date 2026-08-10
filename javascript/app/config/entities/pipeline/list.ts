import { CellType } from '@michelangelo-ai/core';

import { PIPELINE_STATE_CELL, PIPELINE_TYPE_CELL } from './shared';

import type { ColumnConfig, ListViewConfig } from '@michelangelo-ai/core';

export const PIPELINE_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    url: '/${studio.projectId}/${studio.phase}/pipelines/${data.metadata.name}',
    tooltip: {
      content: 'Click to filter by this pipeline name',
      action: 'filter',
    },
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
