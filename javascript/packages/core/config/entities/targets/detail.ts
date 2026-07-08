import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { CONDITION_STATUS, INFERENCE_SERVER_STATE_CELL } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const TARGET_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    {
      id: 'status.createTime',
      label: 'Created',
      type: CellType.DATE,
      accessor: (data: unknown) => {
        // cast: accessor receives unknown data; narrowing to expected proto shape for property
        // access; see #1425
        const ts = (data as { status?: { createTime?: string } })?.status?.createTime;
        return ts ? Math.floor(new Date(ts).getTime() / 1000) : undefined;
      },
    },
    {
      id: 'spec.owner.name',
      label: 'Owner',
      type: CellType.TEXT,
    },
    INFERENCE_SERVER_STATE_CELL,
  ],
  pages: [
    {
      id: 'stages',
      label: 'Stages',
      type: 'execution',
      emptyState: {
        title: 'No stages reported',
        description: 'Stages will appear here once the inference server is initialized',
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
            case CONDITION_STATUS.TRUE:
              return TASK_STATE.SUCCESS;
            case CONDITION_STATUS.FALSE:
              return TASK_STATE.ERROR;
            case CONDITION_STATUS.UNKNOWN:
            default:
              return TASK_STATE.RUNNING;
          }
        },
      },
    },
  ],
};
