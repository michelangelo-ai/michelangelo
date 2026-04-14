import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import { PIPELINE_STATE_CELL, PIPELINE_TYPE_CELL } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const PIPELINE_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT },
    PIPELINE_TYPE_CELL,
    { id: 'spec.commit.branch', label: 'Branch', type: CellType.TEXT },
    PIPELINE_STATE_CELL,
  ],
  pages: [
    {
      id: 'runs',
      label: 'Pipeline Runs',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'pipelineRun',
        serviceOptions: {
          listOptions: {
            // Filter runs to only those belonging to the current pipeline
            labelSelector: 'pipeline.michelangelo/name=${page.metadata.name}',
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
