import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { generateSuffix } from '#core/utils/name-utils';
import { TRIGGER_DETAIL_CONFIG } from './detail';
import { TRIGGER_LIST_CONFIG } from './list';
import { TriggerRunAction, TriggerRunState } from './types';

import type { MiddlewareOperation } from '#core/hooks/use-schema-middleware/types';
import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { TriggerRun } from './types';

const isKillable = (record: unknown) => {
  // cast: record is unknown from the action predicate context; always TriggerRun in this entity
  // config; see #1425
  const state = (record as TriggerRun).status?.state;
  return state === TriggerRunState.RUNNING || state === TriggerRunState.PAUSED;
};

/** States a trigger run no longer progresses out of — the only ones it can be rerun from. */
const TERMINAL_TRIGGER_RUN_STATES: ReadonlySet<TriggerRunState> = new Set([
  TriggerRunState.FAILED,
  TriggerRunState.KILLED,
  TriggerRunState.SUCCEEDED,
]);

const isRerunnable = (record: unknown) => {
  // cast: record is unknown from the action predicate context; always TriggerRun in this entity
  // config; see #1425
  const state = (record as TriggerRun).status?.state;
  return state !== undefined && TERMINAL_TRIGGER_RUN_STATES.has(state);
};

/**
 * Prefixes a rerun's generated name by the kind of trigger it replays, mirroring the
 * priority order the reconciler itself uses to classify a run: a batch rerun stays a batch
 * rerun even with a backfill window set, a backfill window on a cron or interval trigger
 * makes the run a backfill, and otherwise the run is named for its own schedule type. See
 * `pipeline/run-trigger-form.tsx`'s `resolveTriggerRunTypePrefix` for the same precedence
 * applied when a run is started fresh from a trigger definition rather than replayed from
 * an existing run's spec.
 */
function resolveRerunNamePrefix(spec: TriggerRun['spec']): string {
  if (spec.trigger?.triggerType?.case === 'batchRerun') return 'batch-rerun';
  if (spec.startTimestamp && spec.endTimestamp) return 'backfill';
  return spec.trigger?.triggerType?.case === 'intervalSchedule' ? 'interval' : 'cron';
}

/**
 * Builds the create payload for a rerun from the terminated trigger run being replayed.
 *
 * `applyMiddleware` clones the source record first, so the new run inherits its pipeline,
 * revision, schedule, and parameters unchanged; these operations then override only what
 * must differ for the new run — a fresh name, and a clean kill/action state, so a run
 * rerun from an already-killed source doesn't spawn pre-killed.
 */
const RERUN_OPERATIONS: MiddlewareOperation[] = [
  {
    source: 'spec',
    destination: 'metadata.name',
    transformation: (source) => {
      // cast: middleware sources are unknown; this path always holds the source trigger
      // run's spec
      const spec = source as TriggerRun['spec'];
      return `${resolveRerunNamePrefix(spec)}${generateSuffix({ withDate: true })}`;
    },
  },
  { destination: 'spec.action', default: TriggerRunAction.NO_ACTION },
  { destination: 'spec.kill', default: false },
  { destination: 'spec.actor', transformation: 'unset' },
  { destination: 'status', transformation: 'unset' },
];

export const TRIGGER_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'triggers',
  name: 'triggers',
  service: 'triggerRun',
  state: 'active',
  views: [TRIGGER_LIST_CONFIG, TRIGGER_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Kill', icon: 'stopCircle' },
      hierarchy: interpolate(({ data }) =>
        isKillable(data) ? ActionHierarchy.SECONDARY : ActionHierarchy.TERTIARY
      ),
      disabled: [
        {
          condition: interpolate(({ data }) => !isKillable(data)),
          message: 'Only running or paused trigger runs can be killed',
        },
      ],
      operation: {
        type: 'mutation',
        mutation: {
          mutationName: 'UpdateTriggerRun',
          // status.state is set by a controller after the spec change is reconciled.
          // Auto-invalidation runs immediately and refetches stale state; this delayed
          // re-invalidation gives the backend time to process the kill so the next
          // refetch shows PENDING_KILL / KILLED.
          successOperations: [
            {
              type: 'invalidate',
              targets: ['GetTriggerRun', 'ListTriggerRun'],
              delayMs: 2000,
            },
          ],
          middleware: {
            operations: [{ destination: 'spec.action', default: TriggerRunAction.KILL }],
          },
        },
      },
      modal: {
        type: 'confirm',
        header: { title: 'Kill Trigger Run' },
        body: interpolate(
          ({ data }) =>
            // cast: data is unknown from interpolation context; always TriggerRun in this entity
            // config; see #1425
            `Kill run **${(data as TriggerRun).metadata.name}** in pipeline **${(data as TriggerRun).spec.pipeline.name}**? This action cannot be undone.`
        ),
        button: { label: 'Kill' },
      },
    },
    {
      display: { label: 'Rerun', icon: 'playerPlay' },
      hierarchy: ActionHierarchy.SECONDARY,
      disabled: [
        {
          condition: interpolate(({ data }) => !isRerunnable(data)),
          message: 'Only terminated trigger runs (failed, killed, or succeeded) can be rerun',
        },
      ],
      operation: {
        type: 'mutation',
        mutation: {
          mutationName: 'CreateTriggerRun',
          successOperations: [
            { type: 'invalidate', targets: ['ListTriggerRun'] },
            {
              type: 'toast',
              message: 'A new trigger run has been created for this pipeline revision.',
              action: {
                label: 'See new trigger',
                route: interpolate(
                  '/${studio.projectId}/${studio.phase}/triggers/${response.triggerRun.metadata.name}'
                ),
              },
            },
          ],
          middleware: { operations: RERUN_OPERATIONS },
        },
      },
      modal: {
        type: 'confirm',
        header: { title: 'Rerun Trigger' },
        body: interpolate(
          ({ data }) =>
            // cast: data is unknown from interpolation context; always TriggerRun in this entity
            // config; see #1425
            `Rerun **${(data as TriggerRun).metadata.name}**? This creates a new trigger run using the same pipeline, revision, and schedule.`
        ),
        button: { label: 'Rerun' },
      },
    },
  ],
};
