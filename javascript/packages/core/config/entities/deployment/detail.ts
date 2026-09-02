import { CellType } from '#core/components/cell/constants';
import { TAG_COLOR } from '#core/components/tag/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { DeploymentInfoPage } from './deployment-info-page';
import {
  DEPLOYMENT_CONDITION_STATUS,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
  FAILED_ROLLOUT_STAGES,
} from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

/**
 * State-cell keys per condition status. CONDITION_STATUS_UNKNOWN is 0, which
 * the state cell drops as a missing value — map to string keys so every
 * status renders.
 */
const CONDITION_STATUS_KEYS: Record<number, string> = {
  [DEPLOYMENT_CONDITION_STATUS.TRUE]: 'CONDITION_STATUS_TRUE',
  [DEPLOYMENT_CONDITION_STATUS.FALSE]: 'CONDITION_STATUS_FALSE',
  [DEPLOYMENT_CONDITION_STATUS.UNKNOWN]: 'CONDITION_STATUS_UNKNOWN',
};

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
          const conditions =
            isFailedRollout(status?.stage) && hasSnapshot
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
              id: 'status',
              label: 'State',
              type: CellType.STATE,
              accessor: (record: { status?: number }) =>
                CONDITION_STATUS_KEYS[record.status ?? DEPLOYMENT_CONDITION_STATUS.UNKNOWN] ??
                'CONDITION_STATUS_UNKNOWN',
              stateTextMap: {
                CONDITION_STATUS_TRUE: 'Succeeded',
                CONDITION_STATUS_FALSE: 'Running',
                CONDITION_STATUS_UNKNOWN: 'Pending',
              },
              stateColorMap: {
                CONDITION_STATUS_TRUE: TAG_COLOR.green,
                CONDITION_STATUS_FALSE: TAG_COLOR.blue,
                CONDITION_STATUS_UNKNOWN: TAG_COLOR.gray,
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
