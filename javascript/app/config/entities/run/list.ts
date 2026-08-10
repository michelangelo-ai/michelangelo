import { CellType } from '@michelangelo-ai/core';

import { SHARED_RUN_CELL_CONFIG } from './shared';

import type { ColumnConfig, ListViewConfig } from '@michelangelo-ai/core';

export const PIPELINE_RUN_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    items: [
      {
        id: 'metadata.name',
        url: '/${studio.projectId}/${studio.phase}/runs/${data.metadata.name}',
      },
      {
        id: 'spec.description',
        type: CellType.DESCRIPTION,
      },
    ],
  },
  ...SHARED_RUN_CELL_CONFIG,
];

export const RUN_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: PIPELINE_RUN_CELL_CONFIG,
  },
};
