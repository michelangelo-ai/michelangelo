import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import { TRIGGER_STATE_CELL_CONFIG } from '#core/config/entities/trigger/shared';
import {
  CRITERION_OPERATOR_EQUAL,
  PIPELINE_RUN_PIPELINE_NAME_FIELD,
  PIPELINE_STATE_CELL,
  PIPELINE_TYPE_CELL,
} from './shared';

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
      label: 'Runs',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'pipelineRun',
        serviceOptions: {
          listOptionsExt: {
            operation: {
              criterion: [
                {
                  fieldName: PIPELINE_RUN_PIPELINE_NAME_FIELD,
                  operator: CRITERION_OPERATOR_EQUAL,
                  matchValue: '${page.metadata.name}',
                },
              ],
            },
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
    {
      id: 'triggers',
      label: 'Triggers',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'triggerRun',
        serviceOptions: {
          listOptions: {
            // Same label convention as the Pipeline Runs tab above, scoped one level up:
            // every TriggerRun references its pipeline via `spec.pipeline` (see
            // TriggerRunSpec.pipeline in proto/api/v2/trigger_run.proto).
            labelSelector: 'pipeline.michelangelo/name=${page.metadata.name}',
          },
        },
      },
      tableConfig: {
        columns: [
          {
            id: 'metadata.name',
            label: 'Name',
            url: '/${studio.projectId}/${studio.phase}/triggers/${row.metadata.name}',
          },
          { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
          { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
          { id: 'spec.trigger.triggerType.value.cron', label: 'Cron', type: CellType.TEXT },
          {
            id: 'spec.trigger.triggerType.value.interval.seconds',
            label: 'Interval seconds',
            type: CellType.TEXT,
          },
          TRIGGER_STATE_CELL_CONFIG,
        ],
      },
    },
  ],
};
