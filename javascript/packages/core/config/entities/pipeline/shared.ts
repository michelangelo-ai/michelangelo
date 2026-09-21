import { ActionHierarchy } from '#core/components/actions/types';
import { CellType } from '#core/components/cell/constants';
import { interpolate } from '#core/interpolation/interpolate';
import { getCrdLastUpdatedSeconds } from '#core/utils/crd-utils';
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

/**
 * Shared by the pipeline and revision lists. Reads the apiserver's `michelangelo/UpdateTimestamp`
 * label, falling back to creation time for rows that have never been updated.
 */
export const PIPELINE_LAST_UPDATED_CELL: Cell = {
  id: 'metadata',
  label: 'Last updated',
  type: CellType.DATE,
  accessor: (data: unknown) => {
    // cast: accessor receives unknown data; narrowing to expected proto shape for property
    // access; see #1425
    const row = data as {
      metadata?: { labels?: Record<string, string>; creationTimestamp?: { seconds: number } };
    };
    return getCrdLastUpdatedSeconds(row);
  },
};

/**
 * Mirrors the generated proto PipelineState enum (pipeline.proto). Colocated here until core
 * has access to the shared generated package — swapping the import path is the only change
 * needed then, since usage sites reference `PipelineState.READY` etc.
 */
export const PipelineState = {
  INVALID: 'PIPELINE_STATE_INVALID',
  CREATED: 'PIPELINE_STATE_CREATED',
  BUILDING: 'PIPELINE_STATE_BUILDING',
  READY: 'PIPELINE_STATE_READY',
  ERROR: 'PIPELINE_STATE_ERROR',
} as const;

/**
 * Mirrors the generated proto PipelineType enum (pipeline.proto). See PipelineState above.
 */
export const PipelineType = {
  INVALID: 'PIPELINE_TYPE_INVALID',
  TRAIN: 'PIPELINE_TYPE_TRAIN',
  EVAL: 'PIPELINE_TYPE_EVAL',
  PERF_EVAL: 'PIPELINE_TYPE_PERF_EVAL',
  EXPERIMENT: 'PIPELINE_TYPE_EXPERIMENT',
  RETRAIN: 'PIPELINE_TYPE_RETRAIN',
  PREDICTION: 'PIPELINE_TYPE_PREDICTION',
  PERFORMANCE_MONITORING: 'PIPELINE_TYPE_PERFORMANCE_MONITORING',
  BASIS_FEATURE: 'PIPELINE_TYPE_BASIS_FEATURE',
  DATA_PREP: 'PIPELINE_TYPE_DATA_PREP',
  ONLINE_OFFLINE_FEATURE_CONSISTENCY: 'PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY',
  FEATURE_GROUP_COMPUTE: 'PIPELINE_TYPE_FEATURE_GROUP_COMPUTE',
  ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION:
    'PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION',
  POST_PROCESSING: 'PIPELINE_TYPE_POST_PROCESSING',
  OPTIMIZATION: 'PIPELINE_TYPE_OPTIMIZATION',
  SCORER: 'PIPELINE_TYPE_SCORER',
} as const;

export const PIPELINE_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    [PipelineState.INVALID]: 'Invalid',
    [PipelineState.CREATED]: 'Created',
    [PipelineState.BUILDING]: 'Building',
    [PipelineState.READY]: 'Ready',
    [PipelineState.ERROR]: 'Error',
  },
  stateColorMap: {
    [PipelineState.INVALID]: 'red',
    [PipelineState.CREATED]: 'green',
    [PipelineState.BUILDING]: 'yellow',
    [PipelineState.READY]: 'green',
    [PipelineState.ERROR]: 'red',
  },
};

export const PIPELINE_TYPE_CELL: Cell = {
  id: 'spec.type',
  label: 'Type',
  type: CellType.TYPE,
  typeTextMap: {
    [PipelineType.INVALID]: 'Invalid',
    [PipelineType.TRAIN]: 'Train',
    [PipelineType.EVAL]: 'Evaluation',
    [PipelineType.PERF_EVAL]: 'Performance Evaluation',
    [PipelineType.EXPERIMENT]: 'Experiment',
    [PipelineType.RETRAIN]: 'Retrain',
    [PipelineType.PREDICTION]: 'Prediction',
    [PipelineType.PERFORMANCE_MONITORING]: 'Performance Monitoring',
    [PipelineType.BASIS_FEATURE]: 'Basis Feature',
    [PipelineType.DATA_PREP]: 'Data Prep',
    [PipelineType.ONLINE_OFFLINE_FEATURE_CONSISTENCY]: 'Online Offline Feature Consistency',
    [PipelineType.FEATURE_GROUP_COMPUTE]: 'Feature Group Compute',
    [PipelineType.ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION]:
      'Online Offline Feature Consistency Orchestration',
    [PipelineType.POST_PROCESSING]: 'Post Processing',
    [PipelineType.OPTIMIZATION]: 'Optimization',
    [PipelineType.SCORER]: 'Scorer',
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
      return `Delete pipeline **${name}**? This will delete the pipeline and all of its revisions. This action cannot be undone.`;
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
