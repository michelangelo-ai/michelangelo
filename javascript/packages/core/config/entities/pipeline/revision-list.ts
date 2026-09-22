import { CellType } from '#core/components/cell/constants';
import { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';
import { formatRevisionLabel } from '#core/utils/revision-utils';
import { PIPELINE_STATE_CELL, PIPELINE_TYPE_CELL } from './shared';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableConfig } from '#core/components/views/types';
import type { PipelineRevision } from './types';

/**
 * Columns for the Revisions variant of the pipeline list. Mirrors the pipeline columns
 * so the two views line up when toggled, with pipeline-derived cells (type, state) read
 * from the wrapped Pipeline in `spec.content`.
 */
export const PIPELINE_REVISION_CELL_CONFIG: ColumnConfig<object>[] = [
  {
    id: 'spec.baseResource.name',
    label: 'Name',
    type: CellType.MULTI,
    items: [
      {
        id: 'spec.baseResource.name',
        url: '/${studio.projectId}/${studio.phase}/pipelines/${data.spec.baseResource.name}?revisionId=${data.spec.revisionId}',
      },
      {
        id: 'spec.revisionId',
        type: CellType.DESCRIPTION,
        hierarchy: DescriptionHierarchy.PRIMARY,
        // cast: accessor rows are untyped in table config; always a PipelineRevision on this
        // variant's query; see #1425
        accessor: (row: unknown) => formatRevisionLabel((row as PipelineRevision).spec?.revisionId),
      },
    ],
  },
  { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
  { ...PIPELINE_TYPE_CELL, id: 'spec.content.spec.type' },
  { id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT },
  { id: 'spec.gitCommit.branch', label: 'Branch', type: CellType.TEXT },
  { ...PIPELINE_STATE_CELL, id: 'spec.content.status.state' },
];

export const PIPELINE_REVISION_TABLE_CONFIG: TableConfig<object> = {
  columns: PIPELINE_REVISION_CELL_CONFIG,
  actions: [],
};
