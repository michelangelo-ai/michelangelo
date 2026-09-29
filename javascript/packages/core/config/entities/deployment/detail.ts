import { CellType } from '#core/components/cell/constants';
import { TAG_COLOR } from '#core/components/tag/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { DeploymentInfoPage } from './deployment-info-page';
import {
  DEPLOYMENT_CONDITION_STATUS,
  DEPLOYMENT_STAGE,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
  FAILED_ROLLOUT_STAGES,
} from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const DEPLOYMENT_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT },
    DEPLOYMENT_STAGE_CELL,
    DEPLOYMENT_STATE_CELL,
  ],
  pages: [
    {
      id: 'info',
      label: 'Information',
      type: 'custom',
      component: DeploymentInfoPage,
    },
    {
      id: 'ongoing-operations',
      label: 'Ongoing operations',
      type: 'execution',
      emptyState: {
        title: 'No deployment rollout in progress',
        description: 'Ongoing operations will appear here when a deployment rollout is in progress',
      },
      tasks: {
        accessor: (data: {
          status?: { stage?: number; conditions?: object[]; conditionsSnapshot?: object[] };
        }) => {
          const status = data?.status;
          const hasSnapshot = (status?.conditionsSnapshot?.length ?? 0) > 0;
          // The controller only snapshots ROLLOUT_FAILED; a ROLLBACK_FAILED deployment's
          // snapshot (if any) is left over from an earlier rollout failure, so read live
          // conditions for it instead.
          const conditions =
            status?.stage === DEPLOYMENT_STAGE.ROLLOUT_FAILED && hasSnapshot
              ? status?.conditionsSnapshot
              : status?.conditions;
          return conditions ?? [];
        },
        header: {
          heading: 'type',
          metadata: [
            {
              id: 'lastUpdatedTimestamp',
              label: 'Last updated',
              type: CellType.DATE,
              accessor: (record: { lastUpdatedTimestamp?: string | number | bigint }) => {
                const ts = record.lastUpdatedTimestamp;
                return ts ? Math.floor(Number(ts) / 1000) : undefined;
              },
            },
            {
              // taskState is the stateBuilder result, injected by TaskHeader, so the
              // chip always agrees with the task icon and card color.
              id: 'taskState',
              label: 'State',
              type: CellType.STATE,
              stateTextMap: {
                [TASK_STATE.SUCCESS]: 'Succeeded',
                [TASK_STATE.RUNNING]: 'Running',
                [TASK_STATE.PENDING]: 'Pending',
                [TASK_STATE.ERROR]: 'Failed',
                [TASK_STATE.SKIPPED]: 'Skipped',
              },
              stateColorMap: {
                [TASK_STATE.SUCCESS]: TAG_COLOR.green,
                [TASK_STATE.RUNNING]: TAG_COLOR.blue,
                [TASK_STATE.PENDING]: TAG_COLOR.gray,
                [TASK_STATE.ERROR]: TAG_COLOR.red,
                [TASK_STATE.SKIPPED]: TAG_COLOR.gray,
              },
            },
          ],
        },
        body: [
          {
            type: 'textarea',
            label: 'Information',
            accessor: 'message',
            markdown: false,
          },
          {
            type: 'textarea',
            label: 'Details',
            accessor: 'reason',
            markdown: false,
          },
        ],
        stateBuilder: (
          record: { status: number },
          index: number,
          siblings: { status: number }[],
          data: { status?: { stage?: number } }
        ) => {
          if (record.status === DEPLOYMENT_CONDITION_STATUS.TRUE) return TASK_STATE.SUCCESS;

          // The first incomplete condition is the step the controller is working
          // on: running while the rollout is healthy, the failure point otherwise.
          // Conditions behind it haven't been reached yet.
          const isFirstIncomplete =
            siblings.findIndex((s) => s.status !== DEPLOYMENT_CONDITION_STATUS.TRUE) === index;

          if (isFirstIncomplete) {
            return isFailedRollout(data.status?.stage) ? TASK_STATE.ERROR : TASK_STATE.RUNNING;
          }
          return TASK_STATE.PENDING;
        },
      },
    },
  ],
};

function isFailedRollout(stage: number | undefined): boolean {
  return stage != null && FAILED_ROLLOUT_STAGES.includes(stage);
}
