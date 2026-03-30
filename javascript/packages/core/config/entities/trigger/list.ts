import { CellType } from '#core/components/cell/constants';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';

import type { ListViewConfig } from '#core/components/views/types';

export const TRIGGER_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: [
      {
        id: 'metadata.name',
        label: 'Name',
        url: '/${studio.projectId}/${studio.phase}/triggers/${data.metadata.name}',
      },
      { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
      { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
      TRIGGER_PIPELINE_CELL_CONFIG,
      TRIGGER_STATE_CELL_CONFIG,
      { id: 'data', label: 'Kill', type: CellType.TRIGGER_KILL },
      { id: 'data', label: 'Pause', type: CellType.TRIGGER_PAUSE },
      { id: 'data', label: 'Resume', type: CellType.TRIGGER_RESUME },
    ],
  },
};
