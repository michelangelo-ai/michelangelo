import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

export const PIPELINE_TYPE_TEXT_MAP: Record<number, string> = {
  0: 'Invalid',
  1: 'Train',
  2: 'Evaluation',
  3: 'Performance Evaluation',
  4: 'Experiment',
  5: 'Retrain',
  6: 'Prediction',
  7: 'Performance Monitoring',
  8: 'Basis Feature',
  9: 'Data Prep',
  10: 'Online Offline Feature Consistency',
  11: 'Feature Group Compute',
  12: 'Online Offline Feature Consistency Orchestration',
  13: 'Post Processing',
  14: 'Optimization',
  15: 'Scorer',
};

export const PIPELINE_STATE_TEXT_MAP: Record<number, string> = {
  0: 'Invalid',
  1: 'Created',
  2: 'Building',
  3: 'Ready',
  4: 'Error',
};

export const PIPELINE_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: PIPELINE_STATE_TEXT_MAP,
  stateColorMap: {
    0: 'red',
    1: 'green',
    2: 'yellow',
    3: 'green',
    4: 'red',
  },
};

export const PIPELINE_TYPE_CELL: Cell = {
  id: 'spec.type',
  label: 'Type',
  type: CellType.TYPE,
  typeTextMap: PIPELINE_TYPE_TEXT_MAP,
};
