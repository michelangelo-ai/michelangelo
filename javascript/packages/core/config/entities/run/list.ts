import { CellType } from '#core/components/cell/constants';
import { getCrdLastUpdatedSeconds } from '#core/utils/crd-utils';
import { readEnvironmentLabel } from '#core/utils/environment-utils';
import { RUN_PIPELINE_COLUMN, RUN_STARTED_BY_COLUMN, RUN_STATE_COLUMN } from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const PIPELINE_RUN_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Pipeline run name',
    items: [
      {
        id: 'metadata.name',
        url: '/${studio.projectId}/${studio.phase}/runs/${data.metadata.name}',
      },
      {
        id: 'spec.description',
        type: CellType.DESCRIPTION,
      },
    ],
  },
  RUN_PIPELINE_COLUMN,
  {
    id: 'metadata',
    label: 'Last Updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property
      // access; see #1425
      const row = data as {
        metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
      };
      return getCrdLastUpdatedSeconds(row);
    },
  },
  {
    id: 'metadata.labels',
    label: 'Environment',
    type: CellType.TEXT,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property
      // access; see #1425
      const labels = (data as { metadata?: { labels?: Record<string, string> } })?.metadata?.labels;
      return readEnvironmentLabel(labels) || null;
    },
  },
  RUN_STARTED_BY_COLUMN,
  RUN_STATE_COLUMN,
];

export const RUN_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: PIPELINE_RUN_CELL_CONFIG,
  },
};
