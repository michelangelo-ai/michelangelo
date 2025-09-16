import { CellType } from '#core/components/cell/constants';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

export const TRIGGER_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
    TRIGGER_PIPELINE_CELL_CONFIG,
    TRIGGER_STATE_CELL_CONFIG,
  ],
  pages: [],
};
