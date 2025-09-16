import { CellType } from '#core/components/cell/constants';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const TRIGGER_LIST_CONFIG: ListViewConfig = {
  type: 'list',
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
  ] as ColumnConfig[],
};
