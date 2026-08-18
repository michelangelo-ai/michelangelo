import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { generateSuffix } from '#core/utils/name-utils';
import { RUN_DETAIL_CONFIG } from './detail';
import { RUN_LIST_CONFIG } from './list';
import { PipelineRunState } from './types';

import type { MiddlewareOperation } from '#core/hooks/use-schema-middleware/types';
import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { PipelineRunSummary } from './types';

/**
 * States worth retrying.
 *
 * A succeeded run has every task cached, so resuming it would reuse all of them and do
 * no work; a run that has not finished has nothing settled to resume from. That leaves
 * the two states where a retry actually re-executes something.
 */
const RETRYABLE_RUN_STATES: ReadonlySet<PipelineRunState> = new Set([
  PipelineRunState.FAILED,
  PipelineRunState.KILLED,
]);

const isRetryable = (record: unknown) => {
  // cast: record is unknown from the action predicate context; always a PipelineRun in this
  // entity config; see #1425
  const state = (record as PipelineRunSummary).status?.state;
  return state !== undefined && RETRYABLE_RUN_STATES.has(state);
};

/**
 * Reshapes the run being retried into the payload that creates its replacement.
 *
 * Retry here is resume-shaped rather than a reset: instead of rewinding the original
 * workflow in place (what `spec.retryInfo` and the per-step Retry button do), it starts a
 * fresh run whose `spec.resume` points back at this one. Every task that already
 * succeeded is then served from cache, so execution effectively picks up at the first
 * task that did not.
 *
 * Order matters — operations are applied in sequence against the same record, so
 * `spec.resume` has to be written from `metadata.name` before that field is replaced with
 * the new run's name.
 *
 * Copies need an identity `transformation`: `applyMiddleware` only writes when a
 * transformation function is present, or when the source is nil and a `default` is set.
 */
const RETRY_AS_RESUME_OPERATIONS: MiddlewareOperation[] = [
  {
    source: 'metadata.name',
    destination: 'spec.resume.pipelineRun.name',
    transformation: (name) => name,
  },
  {
    source: 'metadata.namespace',
    destination: 'spec.resume.pipelineRun.namespace',
    transformation: (namespace) => namespace,
  },
  // Leaving resumeFrom unset reuses every cached success. Inheriting it from a run that
  // was itself resumed would instead force those same tasks to re-run.
  { destination: 'spec.resume.resumeFrom', transformation: 'unset' },
  // Run-specific or server-owned spec fields that must not carry into a new run.
  { destination: 'spec.retryInfo', transformation: 'unset' },
  { destination: 'spec.actor', transformation: 'unset' },
  { destination: 'spec.kill', transformation: 'unset' },
  { destination: 'status', transformation: 'unset' },
  { destination: 'typeMeta', transformation: 'unset' },
  // Rebuilt wholesale rather than field-by-field: the API rejects a create carrying uid or
  // resourceVersion, and creationTimestamp, finalizers, ownerReferences, labels,
  // annotations and managedFields all describe the source run. Runs last so the two
  // operations above still read the original name.
  {
    source: 'metadata',
    destination: 'metadata',
    transformation: (metadata) => ({
      name: `run${generateSuffix({ withDate: true })}`,
      // cast: middleware sources are unknown; this path always holds run metadata
      namespace: (metadata as { namespace: string }).namespace,
    }),
  },
];

export const RUN_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'runs',
  name: 'Pipeline Runs',
  service: 'pipelineRun',
  state: 'active',
  views: [RUN_LIST_CONFIG, RUN_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Retry', icon: 'arrowCircular' },
      hierarchy: ActionHierarchy.PRIMARY,
      disabled: [
        {
          condition: interpolate(({ data }) => !isRetryable(data)),
          message: 'Only failed or killed runs can be retried',
        },
      ],
      operation: {
        type: 'mutation',
        mutation: {
          mutationName: 'CreatePipelineRun',
          middleware: { operations: RETRY_AS_RESUME_OPERATIONS },
          successOperations: [
            { type: 'invalidate', targets: ['ListPipelineRun'] },
            {
              type: 'toast',
              message: 'A new pipeline run has been created from the failed step of this run.',
              action: {
                label: 'See new run',
                route: interpolate(
                  '/${studio.projectId}/${studio.phase}/runs/${response.pipelineRun.metadata.name}'
                ),
              },
            },
          ],
        },
      },
      modal: {
        type: 'confirm',
        header: { title: 'Retry Pipeline Run' },
        body: interpolate(
          ({ data }) =>
            // cast: data is unknown from interpolation context; always a PipelineRun in this
            // entity config; see #1425
            `Retry run **${(data as PipelineRunSummary).metadata?.name}**? This starts a new run that reuses the cached results of every step that already succeeded.`
        ),
        button: { label: 'Retry' },
      },
    },
  ],
};
