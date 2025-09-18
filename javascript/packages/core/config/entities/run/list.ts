import { SHARED_RUN_CELL_CONFIG } from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const PIPELINE_RUN_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Name',
    url: '/${studio.projectId}/${studio.phase}/runs/${data.metadata.name}',
  },
  ...SHARED_RUN_CELL_CONFIG,
];

export const RUN_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: PIPELINE_RUN_CELL_CONFIG,
  },
};
