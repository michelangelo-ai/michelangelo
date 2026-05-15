import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { TRIGGER_DETAIL_CONFIG } from './detail';
import { TRIGGER_LIST_CONFIG } from './list';
import { TriggerRunAction, TriggerRunState } from './types';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { TriggerRun } from './types';

const isKillable = (record: unknown) => {
  const state = (record as TriggerRun).status?.state;
  return state === TriggerRunState.RUNNING || state === TriggerRunState.PAUSED;
};

export const TRIGGER_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'triggers',
  name: 'Triggers',
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
      action: {
        type: 'mutation',
        mutation: { mutationName: 'UpdateTriggerRun' },
        middleware: {
          operations: [{ destination: 'spec.action', default: TriggerRunAction.KILL }],
        },
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
      },
      modal: {
        type: 'confirm',
        header: { title: 'Kill Trigger Run' },
        body: interpolate(({ data }) => {
          const run = data as TriggerRun;
          return (
            <p>
              Kill run <strong>{run.metadata.name}</strong> in pipeline{' '}
              <strong>{run.spec.pipeline.name}</strong>? This action cannot be undone.
            </p>
          );
        }),
        button: { label: 'Kill' },
        destructive: true,
      },
    },
  ],
};
