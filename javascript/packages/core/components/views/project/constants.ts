import { CellType } from '#core/components/cell/constants';

export const SHARED_PROJECT_CELL_CONFIG = [
  {
    id: 'metadata.name',
    label: 'Name',
    url: '${row.metadata.name}',
  },
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
