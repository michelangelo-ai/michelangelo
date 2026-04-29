import { CellType } from '#core/components/cell/constants';
import {
  DEPLOYMENT_LAST_PREDICTION_CELL,
  DEPLOYMENT_STAGE_CELL,
  DEPLOYMENT_STATE_CELL,
} from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

const DEPLOYMENT_COLUMNS: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    type: CellType.TEXT,
  },
  {
    id: 'status.currentRevision.name',
    label: 'Model',
    type: CellType.TEXT,
  },
  {
    id: 'spec.definition.type',
    label: 'Type',
    type: CellType.TAG,
    accessor: (data: unknown) => {
      const type = (data as { spec?: { definition?: { type?: string } } })?.spec?.definition?.type;
      if (!type) return null;
      if (type === 'TARGET_TYPE_OFFLINE') return 'Offline';
      if (type === 'TARGET_TYPE_MOBILE') return 'Mobile';
      return 'Online';
    },
  },
  DEPLOYMENT_STAGE_CELL,
  {
    id: 'spec.inferenceServer.name',
    label: 'Target',
    type: CellType.TEXT,
    accessor: (data: unknown) => {
      const target = (data as { spec?: { target?: { case?: string; value?: { name?: string } } } })
        ?.spec?.target;
      if (target?.case === 'inferenceServer') return target.value?.name ?? null;
      return null;
    },
  },
  DEPLOYMENT_LAST_PREDICTION_CELL,
  {
    id: 'metadata.labels["michelangelo/SpecUpdateTimestamp"]',
    label: 'Last updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      const ts = (
        data as { metadata?: { labels?: { 'michelangelo/SpecUpdateTimestamp'?: string } } }
      )?.metadata?.labels?.['michelangelo/SpecUpdateTimestamp'];
      return ts ? Math.floor(Number(ts) / 1_000_000) : undefined;
    },
  },
  {
    id: 'spec.owner.name',
    label: 'Owner',
    type: CellType.TEXT,
  },
  DEPLOYMENT_STATE_CELL,
];

export const DEPLOYMENT_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: DEPLOYMENT_COLUMNS,
  },
};
