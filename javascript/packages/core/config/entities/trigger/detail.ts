import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import { TriggerInformationTab } from './components/trigger-information-tab';
import {
  TRIGGER_BATCH_SIZE_CELL_CONFIG,
  TRIGGER_CRON_CELL_CONFIG,
  TRIGGER_END_TIME_CELL_CONFIG,
  TRIGGER_INTERVAL_CELL_CONFIG,
  TRIGGER_MAX_CONCURRENCY_CELL_CONFIG,
  TRIGGER_PIPELINE_CELL_CONFIG,
  TRIGGER_START_TIME_CELL_CONFIG,
  TRIGGER_STATE_CELL_CONFIG,
  TRIGGER_WAIT_CELL_CONFIG,
} from './shared';

import type { TriggerRun } from './types';
import type { DetailViewConfig } from '#core/components/views/types';

export const TRIGGER_DETAIL_CONFIG: DetailViewConfig<TriggerRun> = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
    TRIGGER_PIPELINE_CELL_CONFIG,
    TRIGGER_STATE_CELL_CONFIG,
    TRIGGER_CRON_CELL_CONFIG,
    TRIGGER_INTERVAL_CELL_CONFIG,
    TRIGGER_BATCH_SIZE_CELL_CONFIG,
    TRIGGER_WAIT_CELL_CONFIG,
    TRIGGER_MAX_CONCURRENCY_CELL_CONFIG,
    TRIGGER_START_TIME_CELL_CONFIG,
    TRIGGER_END_TIME_CELL_CONFIG,
  ],
  pages: [
    {
      id: 'information',
      label: 'Information',
      type: 'custom',
      component: TriggerInformationTab,
    },
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
