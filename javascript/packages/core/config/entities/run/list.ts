import { CellType } from '#core/components/cell/constants';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const PIPELINE_RUN_CELL_CONFIG: ColumnConfig[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    url: '/${studio.projectId}/runs/${data.metadata.name}',
  },
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
      0: 'gray',
      1: 'blue',
      2: 'blue',
      3: 'green',
      4: 'red',
      5: 'red',
      6: 'gray',
    },
  },
];

export const RUN_LIST_CONFIG: ListViewConfig = {
  type: 'list',
  columns: PIPELINE_RUN_CELL_CONFIG,
};
