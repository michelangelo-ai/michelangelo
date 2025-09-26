import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const TRIGGER_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
    TRIGGER_PIPELINE_CELL_CONFIG,
    TRIGGER_STATE_CELL_CONFIG,
  ],
  pages: [
    {
      id: 'runs',
      label: 'Recent Runs',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'pipelineRun',
        serviceOptions: {
          listOptions: {
            labelSelector: 'pipelinerun.michelangelo/triggered-by=${page.metadata.name}',
          },
        },
      },
      tableConfig: {
        columns: [
          {
            id: 'metadata.name',
            label: 'Name',
            url: '/${studio.projectId}/${studio.phase}/runs/${row.metadata.name}',
          },
          ...SHARED_RUN_CELL_CONFIG,
        ],
      },
    },
  ],
};
