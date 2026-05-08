import { CellType } from '#core/components/cell/constants';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const INFERENCE_SERVER_STATE = {
  INVALID: 0,
  INITIALIZED: 1,
  CREATE_PENDING: 2,
  SERVING: 3,
  FAILED: 4,
  DELETE_PENDING: 5,
  CREATING: 6,
  DELETING: 7,
  DELETED: 8,
} as const;

const TARGET_COLUMNS: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Target name',
    type: CellType.TEXT,
  },
  {
    id: 'status.updateTime',
    label: 'Last updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      const ts = (data as { status?: { updateTime?: string } })?.status?.updateTime;
      return ts ? Math.floor(new Date(ts).getTime() / 1000) : undefined;
    },
  },
  {
    id: 'type',
    label: 'Type',
    type: CellType.TAG,
    accessor: () => 'Inference Server',
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
];

export const TARGET_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: TARGET_COLUMNS,
  },
};
