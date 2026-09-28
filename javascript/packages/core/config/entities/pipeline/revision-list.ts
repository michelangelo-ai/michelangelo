import { CellType } from '#core/components/cell/constants';
import { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';
import { formatRevisionLabel } from '#core/utils/revision-utils';
import {
  PIPELINE_ACTIONS,
  PIPELINE_DELETE_ACTION,
  PIPELINE_LAST_UPDATED_CELL,
  PIPELINE_STATE_CELL,
  PIPELINE_TYPE_CELL,
} from './shared';

import type { ActionConfigSchema } from '#core/components/actions/types';
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
  PIPELINE_LAST_UPDATED_CELL,
  { ...PIPELINE_TYPE_CELL, id: 'spec.content.spec.type' },
  { id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT },
  { id: 'spec.gitCommit.branch', label: 'Branch', type: CellType.TEXT },
  { ...PIPELINE_STATE_CELL, id: 'spec.content.status.state' },
];

/**
 * Same as {@link PIPELINE_ACTIONS}, but Delete is disabled here: deleting a pipeline from
 * the Revisions list would need to resolve which pipeline a revision belongs to and only
 * makes sense from the Pipelines list, where that pipeline is the row itself.
 */
const PIPELINE_REVISION_ACTIONS: ActionConfigSchema<object>[] = PIPELINE_ACTIONS.map((action) =>
  action === PIPELINE_DELETE_ACTION
    ? {
        ...action,
        disabled: [
          {
            condition: true,
            message: 'Return to the Pipelines list view to delete a pipeline',
          },
        ],
      }
    : action
);

export const PIPELINE_REVISION_TABLE_CONFIG: TableConfig<object> = {
  columns: PIPELINE_REVISION_CELL_CONFIG,
  actions: PIPELINE_REVISION_ACTIONS,
};
