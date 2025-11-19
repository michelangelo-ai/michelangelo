import { CellType } from '#core/components/cell/constants';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const MODEL_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    // url: '/${studio.projectId}/${studio.phase}/models/${data.metadata.name}',
    tooltip: {
      content: 'Click to view model details',
      action: 'filter',
    },
  },
  {
    id: 'metadata.creationTimestamp.seconds',
    label: 'Created',
    type: CellType.DATE
  },
  {
    id: 'spec.modelFamily.name',
    label: 'Model Family',
    type: CellType.TEXT,
  },
  {
    id: 'spec.environment',
    label: 'Environment',
    type: CellType.TYPE,
    typeTextMap: {
      0: 'Development',
      1: 'Production',
    },
  },
  {
    id: 'spec.deployed',
    label: 'Deployed',
    type: CellType.BOOLEAN,
  },
  {
    id: 'spec.trainingFramework',
    label: 'Framework',
    type: CellType.TAG,
  },
  {
    id: 'status.state',
    label: 'State',
    type: CellType.STATE,
    stateTextMap: {
      0: 'Draft',
      1: 'Training',
      2: 'Completed',
      3: 'Failed',
      4: 'Deployed',
    },
    stateColorMap: {
      0: 'gray',
      1: 'blue',
      2: 'green',
      3: 'red',
      4: 'green',
    },
  },
];

export const MODEL_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: MODEL_CELL_CONFIG,
  },
};