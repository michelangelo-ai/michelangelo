import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

export const DEPLOYMENT_STAGE_CELL: Cell = {
  id: 'status.stage',
  label: 'Stage',
  type: CellType.TYPE,
  typeTextMap: {
    0: 'Invalid',
    1: 'Validation',
    2: 'Placement',
    3: 'Resource acquisition',
    4: 'Rollout complete',
    5: 'Rollout failed',
    6: 'Rollback in progress',
    7: 'Rollback complete',
    8: 'Rollback failed',
    9: 'Clean up in progress',
    10: 'Clean up complete',
    11: 'Clean up failed',
  },
};

export const DEPLOYMENT_LAST_PREDICTION_CELL: Cell = {
  id: 'metadata.annotations["deployment.michelangelo/last-prediction-timestamp"]',
  label: 'Last prediction',
  type: CellType.DATE,
};

export const DEPLOYMENT_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: { 0: 'Invalid', 1: 'Initializing', 2: 'Healthy', 3: 'Unhealthy', 4: 'Empty' },
  stateColorMap: { 0: 'gray', 1: 'blue', 2: 'green', 3: 'red', 4: 'gray' },
};
