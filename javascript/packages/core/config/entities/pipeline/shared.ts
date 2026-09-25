import { ActionHierarchy } from '#core/components/actions/types';
import { CellType } from '#core/components/cell/constants';
import { interpolate } from '#core/interpolation/interpolate';
import { CreatePipelineRunForm } from './create-pipeline-run-form';
import { RunTriggerForm } from './run-trigger-form';
import { isPipelineRevision } from './types';

import type { ActionConfigSchema } from '#core/components/actions/types';
import type { Cell } from '#core/components/cell/types';
import type { Pipeline } from './types';

/** `CRITERION_OPERATOR_EQUAL` from `proto/api/list.proto`. */
export const CRITERION_OPERATOR_EQUAL = 1;

/** Criterion field name for the pipeline a PipelineRun belongs to. */
export const PIPELINE_RUN_PIPELINE_NAME_FIELD = 'pipeline_run.pipeline_name';

/**
 * Criterion field name for the Revision a PipelineRun was pinned to (`spec.revision.name`).
 */
export const PIPELINE_RUN_REVISION_NAME_FIELD = 'pipeline_run.revision_name';

export const PIPELINE_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    0: 'Invalid',
    1: 'Created',
    2: 'Building',
    3: 'Ready',
    4: 'Error',
  },
  stateColorMap: {
    0: 'red',
    1: 'green',
    2: 'yellow',
    3: 'green',
    4: 'red',
  },
};

export const PIPELINE_TYPE_CELL: Cell = {
  id: 'spec.type',
  label: 'Type',
  type: CellType.TYPE,
  typeTextMap: {
    0: 'Invalid',
    1: 'Train',
    2: 'Evaluation',
    3: 'Performance Evaluation',
    4: 'Experiment',
    5: 'Retrain',
    6: 'Prediction',
    7: 'Performance Monitoring',
    8: 'Basis Feature',
    9: 'Data Prep',
    10: 'Online Offline Feature Consistency',
    11: 'Feature Group Compute',
    12: 'Online Offline Feature Consistency Orchestration',
    13: 'Post Processing',
    14: 'Optimization',
    15: 'Scorer',
  },
};

/**
 * Deletes the Pipeline a row belongs to. Shared between the Pipelines list/detail and the
 * Revisions list variant: on a Pipeline row `record` has no `spec.baseResource`, so the
 * middleware is a no-op and `metadata` is the Pipeline's own; on a PipelineRevision row
 * (the pipeline detail page, or a Revisions list row), it retargets `metadata` at
 * `spec.baseResource` (the Revision's pointer to the Pipeline it snapshots) so the
 * mutation still deletes the Pipeline.
 */
export const PIPELINE_DELETE_ACTION: ActionConfigSchema<object> = {
  display: { label: 'Delete', icon: 'trashCan' },
  hierarchy: ActionHierarchy.TERTIARY,
  operation: {
    type: 'mutation',
    mutation: {
      mutationName: 'DeletePipeline',
      middleware: {
        operations: [
          { source: 'spec.baseResource', destination: 'metadata', transformation: (v) => v },
        ],
      },
      successOperations: [
        // The Pipeline itself is gone by the time DeletePipeline responds, but cascade-deleting
        // its Revisions is async so an immediate ListRevision refetch can still
        // see the stale row so we add a delay for ListRevision before invalidating.
        { type: 'invalidate', targets: ['ListPipeline'] },
        { type: 'invalidate', targets: ['ListRevision'], delayMs: 2000 },
        { type: 'route', route: '/${studio.projectId}/${studio.phase}/pipelines' },
      ],
    },
  },
  modal: {
    type: 'confirm',
    header: { title: 'Delete Pipeline' },
    body: interpolate(({ data }) => {
      // cast: data is unknown from interpolation context; a live Pipeline from list rows,
      // or the wrapping PipelineRevision from the pipeline detail page or a Revisions list
      // row; see #1425
      const pipeline = data as Pipeline;
      const name = isPipelineRevision(data) ? data.spec.baseResource.name : pipeline.metadata.name;
      return `Delete pipeline **${name}**? This action cannot be undone.`;
    }),
    button: { label: 'Delete' },
    destructive: true,
  },
};

/**
 * A record without a manifest (still loading, or a pipeline registered without one) means
 * "unknown", not "no triggers" — fail open and let the dialog explain an empty trigger list.
 * `record` is a live Pipeline from a "Pipelines" list row, or the wrapping Revision from the
 * pipeline detail page or a Revisions list row — the manifest reads from `spec.content` for
 * the latter.
 */
const hasNoTriggers = (record: unknown): boolean => {
  // cast: record is unknown from the action predicate context; a live Pipeline when it isn't a
  // PipelineRevision; see #1425
  const pipeline = record as Pipeline;
  const manifest = isPipelineRevision(record)
    ? record.spec.content?.spec?.manifest
    : pipeline.spec?.manifest;
  return !!manifest && Object.keys(manifest.triggerMap ?? {}).length === 0;
};

/**
 * The action menu shared by every view of a Pipeline record: the Pipelines list/detail and
 * the Revisions list variant. Each action already handles both row shapes (Pipeline or the
 * wrapping PipelineRevision) via {@link isPipelineRevision}, so the same array works
 * unmodified on all three.
 */
export const PIPELINE_ACTIONS: ActionConfigSchema<object>[] = [
  {
    display: { label: 'Run', icon: 'playerPlay' },
    hierarchy: ActionHierarchy.PRIMARY,
    modal: { type: 'custom', component: CreatePipelineRunForm },
  },
  {
    display: { label: 'Run trigger', icon: 'calendarRepeat' },
    hierarchy: ActionHierarchy.SECONDARY,
    disabled: [
      {
        condition: interpolate(({ data }) => hasNoTriggers(data)),
        message: 'No triggers defined for this pipeline',
      },
    ],
    modal: { type: 'custom', component: RunTriggerForm },
  },
  PIPELINE_DELETE_ACTION,
];
