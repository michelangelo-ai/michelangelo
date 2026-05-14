import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

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

export const CONDITION_STATUS = {
  UNKNOWN: 0,
  TRUE: 1,
  FALSE: 2,
} as const;

export const INFERENCE_SERVER_STATE_CELL: Cell = {
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
    [INFERENCE_SERVER_STATE.CREATING]: 'Initializing',
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
};
