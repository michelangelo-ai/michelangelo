import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { INFERENCE_SERVER_STATE } from './list';

import type { DetailViewConfig } from '#core/components/views/types';

export const CONDITION_STATUS = {
  UNKNOWN: 0,
  TRUE: 1,
  FALSE: 2,
} as const;

export const TARGET_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    {
      id: 'status.createTime',
      label: 'Created',
      type: CellType.DATE,
      accessor: (data: unknown) => {
        const ts = (data as { status?: { createTime?: string } })?.status?.createTime;
        return ts ? Math.floor(new Date(ts).getTime() / 1000) : undefined;
      },
    },
    {
      id: 'spec.owner.name',
      label: 'Owner',
      type: CellType.TEXT,
    },
    {
      id: 'status.state',
      label: 'State',
      type: CellType.STATE,
      stateTextMap: {
        [INFERENCE_SERVER_STATE.INVALID]: 'Invalid',
        [INFERENCE_SERVER_STATE.INITIALIZED]: 'Initialized',
        [INFERENCE_SERVER_STATE.CREATE_PENDING]: 'Create pending',
        [INFERENCE_SERVER_STATE.SERVING]: 'Serving',
        [INFERENCE_SERVER_STATE.FAILED]: 'Failed',
        [INFERENCE_SERVER_STATE.DELETE_PENDING]: 'Delete pending',
        [INFERENCE_SERVER_STATE.CREATING]: 'Creating',
        [INFERENCE_SERVER_STATE.DELETING]: 'Deleting',
        [INFERENCE_SERVER_STATE.DELETED]: 'Deleted',
      },
      stateColorMap: {
        [INFERENCE_SERVER_STATE.INVALID]: 'gray',
        [INFERENCE_SERVER_STATE.INITIALIZED]: 'blue',
        [INFERENCE_SERVER_STATE.CREATE_PENDING]: 'blue',
        [INFERENCE_SERVER_STATE.SERVING]: 'green',
        [INFERENCE_SERVER_STATE.FAILED]: 'red',
        [INFERENCE_SERVER_STATE.DELETE_PENDING]: 'blue',
        [INFERENCE_SERVER_STATE.CREATING]: 'blue',
        [INFERENCE_SERVER_STATE.DELETING]: 'blue',
        [INFERENCE_SERVER_STATE.DELETED]: 'gray',
      },
    },
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
