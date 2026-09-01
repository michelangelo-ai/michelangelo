import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import { TRIGGER_STATE_CELL_CONFIG } from '#core/config/entities/trigger/shared';
import { formatTriggerSchedule } from './format-trigger-schedule';
import {
  CRITERION_OPERATOR_EQUAL,
  PIPELINE_RUN_PIPELINE_NAME_FIELD,
  PIPELINE_STATE_CELL,
  PIPELINE_TYPE_CELL,
} from './shared';

import type { DetailViewConfig } from '#core/components/views/types';
import type { TriggerRun } from '#core/config/entities/trigger/types';

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
            fieldSelector: 'pipeline_name=${page.metadata.name}',
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
          {
            id: 'schedule',
            label: 'Schedule',
            type: CellType.TEXT,
            // `triggerType` is a oneof, so one formatted column covers cron, interval, and
            // batch-rerun triggers instead of a sparse column per case.
            // cast: accessor rows are untyped in table config; always a TriggerRun on this
            // tab's query; see #1425
            accessor: (row: unknown) => formatTriggerSchedule((row as TriggerRun).spec?.trigger),
          },
          TRIGGER_STATE_CELL_CONFIG,
        ],
      },
    },
  ],
};
