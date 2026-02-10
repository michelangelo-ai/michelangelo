import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { SHARED_RUN_CELL_CONFIG } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const RUN_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: SHARED_RUN_CELL_CONFIG,
  pages: [
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
              }) => {
                if (record.startTime?.seconds && record.endTime?.seconds) {
                  const start = parseInt(record.startTime.seconds) * 1000;
                  const end = parseInt(record.endTime.seconds) * 1000;
                  const durationMs = end - start;
                  const durationSec = Math.round(durationMs / 1000);
                  return `${durationSec}s`;
                }
                return null;
              },
            },
            {
              id: 'logUrl',
              label: 'Logs',
            },
            {
              id: 'state',
              label: 'Status',
              type: CellType.STATE,
              stateTextMap: {
                0: 'Pending',
                1: 'Pending',
                2: 'Running',
                3: 'Success',
                4: 'Killed',
                5: 'Failed',
                6: 'Skipped',
              },
              stateColorMap: {
                0: 'gray',
                1: 'blue',
                2: 'blue',
                3: 'green',
                4: 'red',
                5: 'red',
                6: 'gray',
              },
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
  ],
};
