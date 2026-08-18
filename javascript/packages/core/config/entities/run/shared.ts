import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';
import type { TagColor } from '#core/components/tag/types';

/**
 * Labels for `PipelineRunStepState`, keyed by the proto enum value.
 * Shared by the Steps tab and the resume step picker so a step reads
 * the same wherever it appears.
 */
export const STEP_STATE_TEXT_MAP: Record<number, string> = {
  0: 'Pending',
  1: 'Pending',
  2: 'Running',
  3: 'Success',
  4: 'Killed',
  5: 'Failed',
  6: 'Skipped',
};

/** Colors matching {@link STEP_STATE_TEXT_MAP}, keyed by the proto enum value. */
export const STEP_STATE_COLOR_MAP: Record<number, TagColor> = {
  0: 'gray',
  1: 'blue',
  2: 'blue',
  3: 'green',
  4: 'red',
  5: 'red',
  6: 'gray',
};

/**
 * Labels for `PipelineRunState`, keyed by the proto enum value.
 * Distinct from {@link STEP_STATE_TEXT_MAP} — a run reads "Succeeded" where a step
 * reads "Success", and state 0 is a queued run but a pending step.
 */
export const RUN_STATE_TEXT_MAP: Record<number, string> = {
  0: 'Queued',
  1: 'Pending',
  2: 'Running',
  3: 'Succeeded',
  4: 'Killed',
  5: 'Failed',
  6: 'Skipped',
};

/** Colors matching {@link RUN_STATE_TEXT_MAP}, keyed by the proto enum value. */
export const RUN_STATE_COLOR_MAP: Record<number, TagColor> = {
  0: 'gray',
  1: 'blue',
  2: 'blue',
  3: 'green',
  4: 'red',
  5: 'red',
  6: 'gray',
};

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
    stateTextMap: RUN_STATE_TEXT_MAP,
    stateColorMap: RUN_STATE_COLOR_MAP,
  },
];
