import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import {
  DEPLOYMENT_CONDITION_STATUS,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
} from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const DEPLOYMENT_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'metadata.labels["michelangelo/owner"]', label: 'Owner', type: CellType.TEXT },
    DEPLOYMENT_STAGE_CELL,
    DEPLOYMENT_STATE_CELL,
  ],
  pages: [
    {
      id: 'stages',
      label: 'Stages',
      type: 'execution',
      emptyState: {
        title: 'No deployment rollout in progress',
        description: 'Stages will appear here when a deployment rollout is in progress',
      },
      tasks: {
        accessor: 'status.conditions',
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
        stateBuilder: (record: { status: number }) => {
          switch (record.status) {
            case DEPLOYMENT_CONDITION_STATUS.TRUE:
              return TASK_STATE.SUCCESS;
            case DEPLOYMENT_CONDITION_STATUS.FALSE:
              return TASK_STATE.ERROR;
            case DEPLOYMENT_CONDITION_STATUS.UNKNOWN:
            default:
              return TASK_STATE.RUNNING;
          }
        },
      },
    },
  ],
};
