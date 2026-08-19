import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

/**
 * Mirrors the generated proto PipelineRunState enum (pipeline_run.proto). Colocated here until
 * core has access to the shared generated package — swapping the import path is the only change
 * needed then, since usage sites reference `PipelineRunState.READY` etc.
 */
export const PipelineRunState = {
  INVALID: 'PIPELINE_RUN_STATE_INVALID',
  PENDING: 'PIPELINE_RUN_STATE_PENDING',
  RUNNING: 'PIPELINE_RUN_STATE_RUNNING',
  SUCCEEDED: 'PIPELINE_RUN_STATE_SUCCEEDED',
  KILLED: 'PIPELINE_RUN_STATE_KILLED',
  FAILED: 'PIPELINE_RUN_STATE_FAILED',
  SKIPPED: 'PIPELINE_RUN_STATE_SKIPPED',
} as const;

/**
 * Cell configurations rendered for Pipeline Runs:
 *  - Columns for list view
 *  - Header metadata for detail view
 */
export const SHARED_RUN_CELL_CONFIG: Cell[] = [
  { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
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
    id: 'spec.actor.name',
    label: 'Started by',
    type: CellType.TEXT,
  },
  {
    id: 'status.state',
    label: 'State',
    type: CellType.STATE,
    stateTextMap: {
      [PipelineRunState.INVALID]: 'Queued',
      [PipelineRunState.PENDING]: 'Pending',
      [PipelineRunState.RUNNING]: 'Running',
      [PipelineRunState.SUCCEEDED]: 'Succeeded',
      [PipelineRunState.KILLED]: 'Killed',
      [PipelineRunState.FAILED]: 'Failed',
      [PipelineRunState.SKIPPED]: 'Skipped',
    },
    stateColorMap: {
      [PipelineRunState.INVALID]: 'gray',
      [PipelineRunState.PENDING]: 'blue',
      [PipelineRunState.RUNNING]: 'blue',
      [PipelineRunState.SUCCEEDED]: 'green',
      [PipelineRunState.KILLED]: 'red',
      [PipelineRunState.FAILED]: 'red',
      [PipelineRunState.SKIPPED]: 'gray',
    },
  },
];
