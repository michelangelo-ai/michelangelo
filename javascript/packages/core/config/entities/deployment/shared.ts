import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

export const DEPLOYMENT_CONDITION_STATUS = {
  UNKNOWN: 0,
  TRUE: 1,
  FALSE: 2,
} as const;

export const DEPLOYMENT_STAGE = {
  INVALID: 0,
  VALIDATION: 1,
  PLACEMENT: 2,
  RESOURCE_ACQUISITION: 3,
  ROLLOUT_COMPLETE: 4,
  ROLLOUT_FAILED: 5,
  ROLLBACK_IN_PROGRESS: 6,
  ROLLBACK_COMPLETE: 7,
  ROLLBACK_FAILED: 8,
  CLEAN_UP_IN_PROGRESS: 9,
  CLEAN_UP_COMPLETE: 10,
  CLEAN_UP_FAILED: 11,
} as const;

export const DEPLOYMENT_STATE = {
  INVALID: 0,
  INITIALIZING: 1,
  HEALTHY: 2,
  UNHEALTHY: 3,
  EMPTY: 4,
} as const;

export const DEPLOYMENT_STAGE_CELL: Cell = {
  id: 'status.stage',
  label: 'Stage',
  type: CellType.TYPE,
  typeTextMap: {
    [DEPLOYMENT_STAGE.INVALID]: 'Invalid',
    [DEPLOYMENT_STAGE.VALIDATION]: 'Validation',
    [DEPLOYMENT_STAGE.PLACEMENT]: 'Placement',
    [DEPLOYMENT_STAGE.RESOURCE_ACQUISITION]: 'Resource acquisition',
    [DEPLOYMENT_STAGE.ROLLOUT_COMPLETE]: 'Rollout complete',
    [DEPLOYMENT_STAGE.ROLLOUT_FAILED]: 'Rollout failed',
    [DEPLOYMENT_STAGE.ROLLBACK_IN_PROGRESS]: 'Rollback in progress',
    [DEPLOYMENT_STAGE.ROLLBACK_COMPLETE]: 'Rollback complete',
    [DEPLOYMENT_STAGE.ROLLBACK_FAILED]: 'Rollback failed',
    [DEPLOYMENT_STAGE.CLEAN_UP_IN_PROGRESS]: 'Clean up in progress',
    [DEPLOYMENT_STAGE.CLEAN_UP_COMPLETE]: 'Clean up complete',
    [DEPLOYMENT_STAGE.CLEAN_UP_FAILED]: 'Clean up failed',
  },
};

export const DEPLOYMENT_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    [DEPLOYMENT_STATE.INVALID]: 'Invalid',
    [DEPLOYMENT_STATE.INITIALIZING]: 'Initializing',
    [DEPLOYMENT_STATE.HEALTHY]: 'Healthy',
    [DEPLOYMENT_STATE.UNHEALTHY]: 'Unhealthy',
    [DEPLOYMENT_STATE.EMPTY]: 'Empty',
  },
  stateColorMap: {
    [DEPLOYMENT_STATE.INVALID]: 'gray',
    [DEPLOYMENT_STATE.INITIALIZING]: 'blue',
    [DEPLOYMENT_STATE.HEALTHY]: 'green',
    [DEPLOYMENT_STATE.UNHEALTHY]: 'red',
    [DEPLOYMENT_STATE.EMPTY]: 'gray',
  },
};
