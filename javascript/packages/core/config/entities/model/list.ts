import { CellType } from '#core/components/cell/constants';
import { getCrdUpdatedSeconds } from '#core/utils/crd-utils';
import { readEnvironmentLabel } from '#core/utils/environment-utils';
import { MODEL_KIND_TEXT_MAP } from './constants';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ListViewConfig } from '#core/components/views/types';

export const MODEL_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'metadata.name',
    label: 'Model',
    items: [
      {
        id: 'metadata.name',
        url: '/${studio.projectId}/${studio.phase}/models/${data.metadata.name}',
      },
      {
        // spec.description is a free-text summary generated upstream by the training pipeline
        // (e.g. "model workflow=... git=... docker=..."). Rendered verbatim, no parsing.
        id: 'spec.description',
        type: CellType.DESCRIPTION,
      },
    ],
  },
  {
    // Model.spec has no environment field; environment is CRD-label-only metadata, read via
    // readEnvironmentLabel().
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
  {
    id: 'spec.modelFamily.name',
    label: 'Model Family',
    type: CellType.TEXT,
  },
  {
    // See MODEL_KIND_TEXT_MAP above for the full set of ModelKind display labels.
    id: 'spec.kind',
    label: 'Type',
    type: CellType.TYPE,
    typeTextMap: MODEL_KIND_TEXT_MAP,
  },
  {
    // Prefers the SpecUpdateTimestamp label, falls back to creationTimestamp; see
    // getCrdUpdatedSeconds().
    id: 'metadata',
    label: 'Last Updated',
    type: CellType.DATE,
    accessor: (data: unknown) => {
      // cast: accessor receives unknown data; narrowing to expected proto shape for property
      // access; see #1425
      const row = data as {
        metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
      };
      return getCrdUpdatedSeconds(row);
    },
  },
];

export const MODEL_LIST_CONFIG: ListViewConfig<object> = {
  type: 'list',
  tableConfig: {
    columns: MODEL_CELL_CONFIG,
  },
};
