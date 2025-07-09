import { CellType } from '#core/components/cell/constants';

export const SHARED_PROJECT_CELL_CONFIG = [
  {
    id: 'metadata.creationTimestamp.seconds',
    label: 'Created',
    type: CellType.DATE,
  },
  {
    id: 'spec.owner.owningTeam',
    label: 'Owner',
  },
  {
    id: 'spec.tier',
    label: 'Tier',
    type: CellType.TAG,
  },
];

export const PIPELINE_CELL_CONFIG = [
  { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
  {
    id: 'spec.type',
    label: 'Type',
    type: CellType.TYPE,
    typeTextMap: {
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
    },
  },
  {
    id: 'spec.commit.branch',
    label: 'Branch',
    type: CellType.TEXT,
  },
  {
    id: 'status.state',
    label: 'State',
    type: CellType.STATE,
    stateTextMap: {
      0: 'Invalid',
      1: 'Created',
      2: 'Building',
      3: 'Ready',
      4: 'Error',
    },
    stateColorMap: {
      0: 'error',
      1: 'positive',
      2: 'warning',
      3: 'positive',
      4: 'error',
    },
  },
];

export const PIPELINE_RUN_CELL_CONFIG = [
  { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
  {
    id: 'spec.pipeline.name',
    label: 'Pipeline',
    items: [
      {
        id: 'spec.pipeline.name',
        type: CellType.TEXT,
      },
      {
        id: 'spec.revision.name',
        type: CellType.DESCRIPTION,
      },
    ],
  },
  {
    id: 'spec.actor.name',
    label: 'Started by',
    type: CellType.TEXT,
  },
  {
    id: 'status.state',
    label: 'State',
    type: CellType.STATE,
    stateTextMap: {
      0: 'Queued',
      1: 'Pending',
      2: 'Running',
      3: 'Succeeded',
      4: 'Killed',
      5: 'Failed',
      6: 'Skipped',
    },
    stateColorMap: {
      0: 'grey',
      1: 'accent',
      2: 'accent',
      3: 'positive',
      4: 'error',
      5: 'error',
      6: 'grey',
    },
  },
];
