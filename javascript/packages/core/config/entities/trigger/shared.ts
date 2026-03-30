import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

export const TRIGGER_STATE_CELL_CONFIG: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    0: 'Queued',
    1: 'Running',
    2: 'Killed',
    3: 'Failed',
    4: 'Succeeded',
    5: 'Pending Kill',
  },
  stateColorMap: {
    0: 'gray',
    1: 'blue',
    2: 'gray',
    3: 'red',
    4: 'green',
    6: 'yellow',
  },
};

export const TRIGGER_PIPELINE_CELL_CONFIG: Cell = {
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
};
