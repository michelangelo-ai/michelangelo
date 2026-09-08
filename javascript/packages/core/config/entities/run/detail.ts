import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { formatElapsedSeconds } from '#core/utils/time-utils';
import { RunConfigurationPage } from './run-configuration-page';
import { RunInfoPage } from './run-info-page';
import { SHARED_RUN_CELL_CONFIG, STEP_STATE_COLOR_MAP, STEP_STATE_TEXT_MAP } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const RUN_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: SHARED_RUN_CELL_CONFIG,
  pages: [
    {
      id: 'information',
      label: 'Information',
      type: 'custom',
      component: RunInfoPage,
    },
    {
      id: 'steps',
      label: 'Steps',
      type: 'execution',
      emptyState: {
        title: 'No execution data',
        description: 'No steps available for this pipeline run',
      },
      tasks: {
        accessor: 'status.steps',
        subTasksAccessor: 'subSteps',
        header: {
          heading: 'displayName',
          metadata: [
            {
              id: 'startTime.seconds',
              label: 'Start time',
              type: CellType.DATE,
            },
            {
              id: 'endTime.seconds',
              label: 'End time',
              type: CellType.DATE,
            },
            {
              id: 'duration',
              label: 'Duration',
              type: CellType.TEXT,
              accessor: (record: {
                startTime: { seconds: string };
                endTime: { seconds: string };
              }) => formatElapsedSeconds(record.startTime?.seconds, record.endTime?.seconds),
            },
            {
              id: 'logUrl',
              label: 'Logs',
            },
            {
              id: 'state',
              label: 'Status',
              type: CellType.STATE,
              stateTextMap: STEP_STATE_TEXT_MAP,
              stateColorMap: STEP_STATE_COLOR_MAP,
            },
            {
              id: 'retry',
              label: 'Actions',
              type: CellType.RETRY,
              accessor: 'activityId',
              hideEmpty: true,
            },
          ],
        },
        body: [
          {
            type: 'struct',
            label: 'Input Parameters',
            accessor: 'input',
          },
          {
            type: 'struct',
            label: 'Output Results',
            accessor: 'output',
          },
          {
            type: 'textarea',
            label: 'Task Message',
            accessor: 'message',
            markdown: false,
          },
        ],
        stateBuilder: (record: { state: number }) => {
          switch (record.state) {
            case 1:
              return TASK_STATE.PENDING;
            case 2:
              return TASK_STATE.RUNNING;
            case 3:
              return TASK_STATE.SUCCESS;
            case 4:
              return TASK_STATE.ERROR;
            case 5:
              return TASK_STATE.ERROR;
            case 6:
              return TASK_STATE.SKIPPED;
            default:
              return TASK_STATE.PENDING;
          }
        },
      },
    },
    {
      id: 'configuration',
      label: 'Pipeline Configuration',
      type: 'custom',
      component: RunConfigurationPage,
    },
  ],
};
