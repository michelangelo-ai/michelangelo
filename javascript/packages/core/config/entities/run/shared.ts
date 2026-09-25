import { CellType } from '#core/components/cell/constants';
import { interpolate } from '#core/interpolation/interpolate';
import { getCrdExecutionTimestampSeconds, getCrdUpdatedSeconds } from '#core/utils/crd-utils';
import { readEnvironmentLabel } from '#core/utils/environment-utils';
import { PipelineRunState } from './types';

import type { Cell } from '#core/components/cell/types';
import type { TagColor } from '#core/components/tag/types';
import type { RunWithManifest } from './types';

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
export const RUN_TRIGGERED_BY_COLUMN: Cell = {
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
 * Last-updated cell using spec-only semantics (see {@link getCrdUpdatedSeconds}) — distinct
 * from `run/list.ts`'s own "Last Updated" column, which intentionally uses any-update
 * semantics for that page.
 */
export const RUN_UPDATED_COLUMN: Cell = {
  // Distinct from RUN_EXECUTION_TIMESTAMP_COLUMN's id below — both cells read `metadata` in
  // their accessor, but column ids must be unique within a table, so a plain `'metadata'`
  // (which would collide once both cells are used together, e.g. on the trigger detail page)
  // isn't usable for either.
  id: 'metadata-last-updated',
  label: 'Last updated',
  type: CellType.DATE,
  accessor: (data: unknown) => {
    // cast: accessor receives unknown data; narrowing to expected proto shape for property
    // access
    const row = data as {
      metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
    };
    return getCrdUpdatedSeconds(row);
  },
};

/** Raw pipeline-run parameter-id label, blank when the run wasn't parameterized. */
export const RUN_PARAMETER_ID_COLUMN: Cell = {
  id: `metadata.labels['pipelinerun.michelangelo/parameter-id']`,
  label: 'Parameter ID',
  type: CellType.TEXT,
};

/** Execution-timestamp cell; see {@link getCrdExecutionTimestampSeconds} for its fallback rule. */
export const RUN_EXECUTION_TIMESTAMP_COLUMN: Cell = {
  id: 'metadata-execution-timestamp',
  label: 'Execution Timestamp',
  type: CellType.DATE,
  accessor: (data: unknown) => {
    // cast: accessor receives unknown data; narrowing to expected proto shape for property
    // access
    const row = data as {
      metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
    };
    return getCrdExecutionTimestampSeconds(row);
  },
};

/** Normalized environment label, blank when absent or unrecognized. */
export const RUN_ENVIRONMENT_COLUMN: Cell = {
  id: 'metadata.labels',
  label: 'Environment',
  type: CellType.TEXT,
  accessor: (data: unknown) => {
    // cast: accessor receives unknown data; narrowing to expected proto shape for property
    // access
    const labels = (data as { metadata?: { labels?: Record<string, string> } })?.metadata?.labels;
    return readEnvironmentLabel(labels) || null;
  },
};

/** Name of the run this run resumed from, blank for a non-resume run. */
export const RUN_RESUME_FROM_COLUMN: Cell = {
  id: 'spec.resume.pipelineRun.name',
  label: 'Resume from',
  type: CellType.TEXT,
};

/**
 * {@link RUN_STATE_TEXT_MAP} extended with a synthetic "Killing" entry, keyed by the string
 * sentinel {@link RUN_STATE_COLUMN_WITH_KILLING}'s accessor returns — never collides with the
 * real numeric `PipelineRunState` values this map is otherwise keyed by.
 */
export const RUN_STATE_TEXT_MAP_WITH_KILLING: Record<number | string, string> = {
  ...RUN_STATE_TEXT_MAP,
  KILLING: 'Killing',
};

/** Colors matching {@link RUN_STATE_TEXT_MAP_WITH_KILLING}. */
export const RUN_STATE_COLOR_MAP_WITH_KILLING: Record<number | string, TagColor> = {
  ...RUN_STATE_COLOR_MAP,
  // Matches the existing precedent for an in-progress/transitional state: `trigger/shared.ts`'s
  // TRIGGER_STATE_CELL_CONFIG uses 'yellow' for its analogous "Pending Kill" state.
  KILLING: 'yellow',
};

/**
 * Run state cell that injects a synthetic "Killing" value when a kill has been requested
 * (`spec.kill`) but the run's real state hasn't caught up to `KILLED` yet.
 */
export const RUN_STATE_COLUMN_WITH_KILLING: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: RUN_STATE_TEXT_MAP_WITH_KILLING,
  stateColorMap: RUN_STATE_COLOR_MAP_WITH_KILLING,
  accessor: (data: unknown) => {
    // cast: accessor receives unknown data; narrowing to expected proto shape for property
    // access
    const run = data as { spec?: { kill?: boolean }; status?: { state?: number } };
    if (run.spec?.kill && run.status?.state !== PipelineRunState.KILLED) {
      return 'KILLING';
    }
    return run.status?.state;
  },
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
  RUN_TRIGGERED_BY_COLUMN,
  RUN_STATE_COLUMN,
];

/**
 * Resolves the pipeline configuration a run executes, unpacked from the manifest's
 * `content` envelope by the rpc layer. Prefers the snapshot captured when the run started
 * (`status.sourcePipeline`) and falls back to the inline dev-run spec before that snapshot
 * has been resolved. Shared by the Information and Pipeline Configuration tabs.
 */
export function getRunManifestContent(
  run: RunWithManifest | undefined
): Record<string, unknown> | undefined {
  const sourcePipeline = run?.status?.sourcePipeline;
  const manifest =
    sourcePipeline?.pipeline?.spec?.manifest ??
    sourcePipeline?.draftPipeline?.spec?.manifest ??
    run?.spec?.pipelineSpec?.manifest;
  return manifest?.content?.value;
}
