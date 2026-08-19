import { CellType } from '#core/components/cell/constants';
import { interpolate } from '#core/interpolation/interpolate';

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

/** Created-date cell, shared between the run list and detail pages. */
export const RUN_CREATED_COLUMN: Cell = {
  id: 'metadata.creationTimestamp.seconds',
  label: 'Created',
  type: CellType.DATE,
};

/** Pipeline (and revision) cell, shared between the run list and detail pages. */
export const RUN_PIPELINE_COLUMN: Cell = {
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
};

/** Run actor cell, shared between the run list and detail pages. */
export const RUN_STARTED_BY_COLUMN: Cell = {
  id: 'spec.actor.name',
  label: 'Started by',
  type: CellType.TEXT,
};

/** Run state cell, shared between the run list and detail pages. */
export const RUN_STATE_COLUMN: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: RUN_STATE_TEXT_MAP,
  stateColorMap: RUN_STATE_COLOR_MAP,
};

/**
 * Label stamped on every pipeline run spawned by a trigger, holding the name of the
 * originating TriggerRun. Written by the trigger workflow — keep in sync with
 * `TriggerredByLabel` in go/worker/workflows/trigger/cron_trigger_workflows.go.
 *
 * Doubles as the filter key for listing the runs a trigger produced, via
 * `listOptions.labelSelector`.
 */
export const TRIGGERED_BY_LABEL = 'pipelinerun.michelangelo/triggered-by';

/**
 * Links a pipeline run back to the trigger that spawned it.
 *
 * Manually started runs carry no trigger label, so the value renders empty and the URL
 * resolves to '' — which {@link LinkCell} renders as plain text rather than a dead link.
 * That case is the majority, so it is handled explicitly here instead of relying on a
 * failed string interpolation.
 */
export const TRIGGERED_BY_CELL_CONFIG: Cell = {
  id: `metadata.labels['${TRIGGERED_BY_LABEL}']`,
  label: 'Triggered by',
  type: CellType.LINK,
  url: interpolate<string>(({ studio, data }) => {
    // cast: data is `any` from the interpolation context — row in list views, page in
    // detail views; always a PipelineRun for these cells. See #1425
    const triggerName = (data as { metadata?: { labels?: Record<string, string> } })?.metadata
      ?.labels?.[TRIGGERED_BY_LABEL];

    return triggerName ? `/${studio.projectId}/${studio.phase}/triggers/${triggerName}` : '';
  }),
};

/**
 * Cell configurations rendered for Pipeline Runs:
 *  - Columns for list view
 *  - Header metadata for detail view
 */
export const SHARED_RUN_CELL_CONFIG: Cell[] = [
  RUN_CREATED_COLUMN,
  RUN_PIPELINE_COLUMN,
  RUN_STARTED_BY_COLUMN,
  TRIGGERED_BY_CELL_CONFIG,
  RUN_STATE_COLUMN,
];
