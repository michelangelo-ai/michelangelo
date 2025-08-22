import { CellType } from '#core/components/cell/constants';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const PIPELINE_CELL_CONFIG: ColumnConfig[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    url: '/${studio.projectId}/pipelines/${data.metadata.name}',
    tooltip: {
      content: 'Click to filter by this pipeline name',
      action: 'filter',
    },
  },
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
      0: 'red',
      1: 'green',
      2: 'yellow',
      3: 'green',
      4: 'red',
    },
  },
];

export const PIPELINE_LIST_CONFIG: ListViewConfig = {
  type: 'list',
  columns: PIPELINE_CELL_CONFIG,
};
