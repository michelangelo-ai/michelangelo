import { CellType } from '#core/components/cell/constants';
import { INFERENCE_SERVER_STATE_CELL } from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

const TARGET_COLUMNS: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Target name',
    type: CellType.TEXT,
    url: '/${studio.projectId}/${studio.phase}/targets/${row.metadata.name}',
  },
  {
    id: 'status.updateTime',
    label: 'Last updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property
      // access; see #1425
      const ts = (data as { status?: { updateTime?: string } })?.status?.updateTime;
      return ts ? Math.floor(new Date(ts).getTime() / 1000) : undefined;
    },
  },
  {
    id: 'type',
    label: 'Type',
    type: CellType.TAG,
    accessor: () => 'Inference Server',
  },
  INFERENCE_SERVER_STATE_CELL,
];

export const TARGET_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: TARGET_COLUMNS,
  },
};
