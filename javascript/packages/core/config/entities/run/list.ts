import { CellType } from '#core/components/cell/constants';
import { getCrdUpdatedSeconds } from '#core/utils/crd-utils';
import { readEnvironmentLabel } from '#core/utils/environment-utils';
import { RUN_STATE_COLOR_MAP, RUN_STATE_TEXT_MAP } from './shared';

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
    id: 'metadata',
    label: 'Last Updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property access; see #1425
      const row = data as {
        metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
      };
      return getCrdUpdatedSeconds(row);
    },
  },
  { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
  {
    id: 'metadata.labels',
    label: 'Environment',
    type: CellType.TEXT,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property access; see #1425
      const labels = (data as { metadata?: { labels?: Record<string, string> } })?.metadata?.labels;
      return readEnvironmentLabel(labels) || null;
    },
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
    stateTextMap: RUN_STATE_TEXT_MAP,
    stateColorMap: RUN_STATE_COLOR_MAP,
  },
];

export const RUN_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: PIPELINE_RUN_CELL_CONFIG,
  },
};
