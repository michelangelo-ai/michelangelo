import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { TRIGGER_DETAIL_CONFIG } from './detail';
import { TRIGGER_LIST_CONFIG } from './list';
import { TriggerRunAction, TriggerRunState } from './types';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { TriggerRun } from './types';

const isKillable = (record: unknown) => {
  // cast: record is unknown from the action predicate context; always TriggerRun in this entity
  // config; see #1425
  const state = (record as TriggerRun).status?.state;
  return state === TriggerRunState.RUNNING || state === TriggerRunState.PAUSED;
};

export const TRIGGER_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'triggers',
  name: 'triggers',
  service: 'triggerRun',
  state: 'active',
  views: [TRIGGER_LIST_CONFIG, TRIGGER_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Kill', icon: 'stopCircle' },
      // Kill is the only action on this entity, so it's always shown as the top-level
      // action button (see run.ts's Retry action for the same lone-action convention),
      // rather than demoted to the overflow menu when the run isn't currently killable.
      // Non-killability is expressed via the `disabled` rule below instead.
      hierarchy: ActionHierarchy.PRIMARY,
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
  ],
};
