import { CellType } from '#core/components/cell/constants';
import { interpolate } from '#core/interpolation/interpolate';

import type { ProjectOwnerData } from './types';

const getOwner = (data: unknown) => {
  // cast: accessor/interpolation callbacks receive unknown/any data; narrowing to expected proto shape for property access
  return (data as ProjectOwnerData)?.spec?.owner;
};

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
    accessor: (data: unknown) => getOwner(data)?.team?.displayName ?? getOwner(data)?.owningTeam,
    url: interpolate(({ row }) => getOwner(row)?.team?.url),
  },
  {
    id: 'spec.tier',
    label: 'Tier',
    type: CellType.TAG,
  },
];
