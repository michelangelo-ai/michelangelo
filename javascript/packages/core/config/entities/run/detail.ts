import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { SHARED_RUN_CELL_CONFIG } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

/**
 * Mirrors the generated proto PipelineRunStepState enum (pipeline_run.proto). Colocated here
 * until core has access to the shared generated package — swapping the import path is the only
 * change needed then, since usage sites reference `PipelineRunStepState.SUCCEEDED` etc.
 */
export const PipelineRunStepState = {
  INVALID: 'PIPELINE_RUN_STEP_STATE_INVALID',
  PENDING: 'PIPELINE_RUN_STEP_STATE_PENDING',
  RUNNING: 'PIPELINE_RUN_STEP_STATE_RUNNING',
  SUCCEEDED: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
  KILLED: 'PIPELINE_RUN_STEP_STATE_KILLED',
  FAILED: 'PIPELINE_RUN_STEP_STATE_FAILED',
  SKIPPED: 'PIPELINE_RUN_STEP_STATE_SKIPPED',
} as const;

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
                [PipelineRunStepState.INVALID]: 'Pending',
                [PipelineRunStepState.PENDING]: 'Pending',
                [PipelineRunStepState.RUNNING]: 'Running',
                [PipelineRunStepState.SUCCEEDED]: 'Success',
                [PipelineRunStepState.KILLED]: 'Killed',
                [PipelineRunStepState.FAILED]: 'Failed',
                [PipelineRunStepState.SKIPPED]: 'Skipped',
              },
              stateColorMap: {
                [PipelineRunStepState.INVALID]: 'gray',
                [PipelineRunStepState.PENDING]: 'blue',
                [PipelineRunStepState.RUNNING]: 'blue',
                [PipelineRunStepState.SUCCEEDED]: 'green',
                [PipelineRunStepState.KILLED]: 'red',
                [PipelineRunStepState.FAILED]: 'red',
                [PipelineRunStepState.SKIPPED]: 'gray',
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
        stateBuilder: (record: { state: string }) => {
          switch (record.state) {
            case PipelineRunStepState.PENDING:
              return TASK_STATE.PENDING;
            case PipelineRunStepState.RUNNING:
              return TASK_STATE.RUNNING;
            case PipelineRunStepState.SUCCEEDED:
              return TASK_STATE.SUCCESS;
            case PipelineRunStepState.KILLED:
              return TASK_STATE.ERROR;
            case PipelineRunStepState.FAILED:
              return TASK_STATE.ERROR;
            case PipelineRunStepState.SKIPPED:
              return TASK_STATE.SKIPPED;
            default:
              return TASK_STATE.PENDING;
          }
        },
      },
    },
  ],
};
