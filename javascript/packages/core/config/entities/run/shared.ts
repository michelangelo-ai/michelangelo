import { CellType } from '#core/components/cell/constants';
import { Cell } from '#core/components/cell/types';

/**
 * Cell configurations rendered for Pipeline Runs:
 *  - Columns for list view
 *  - Header metadata for detail view
 */
export const SHARED_RUN_CELL_CONFIG: Cell[] = [
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
