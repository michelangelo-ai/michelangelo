import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { DeploymentInfoPage } from './deployment-info-page';
import {
  DEPLOYMENT_CONDITION_STATUS,
  DEPLOYMENT_STAGE,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
  DEPLOYMENT_TERMINAL_STAGES,
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
          const conditions =
            data?.status?.stage === DEPLOYMENT_STAGE.ROLLOUT_FAILED
              ? data?.status?.conditionsSnapshot
              : data?.status?.conditions;
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
          _index,
          _siblings,
          data: { status?: { stage?: number } }
        ) => {
          switch (record.status) {
            case DEPLOYMENT_CONDITION_STATUS.TRUE:
              return TASK_STATE.SUCCESS;
            case DEPLOYMENT_CONDITION_STATUS.FALSE:
              // A condition is FALSE both while its stage is still in progress and once the
              // deployment has terminally failed — conditionsSnapshot (see the accessor above)
              // means a FALSE condition read here is only ever the latter, since it's swapped
              // in exclusively when status.stage === ROLLOUT_FAILED. For any other stage, a
              // FALSE condition just hasn't been satisfied yet, not failed.
              return DEPLOYMENT_TERMINAL_STAGES.has(data?.status?.stage ?? DEPLOYMENT_STAGE.INVALID)
                ? TASK_STATE.ERROR
                : TASK_STATE.RUNNING;
            case DEPLOYMENT_CONDITION_STATUS.UNKNOWN:
            default:
              return TASK_STATE.RUNNING;
          }
        },
      },
    },
  ],
};
