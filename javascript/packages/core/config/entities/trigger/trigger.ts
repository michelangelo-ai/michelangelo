import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { TRIGGER_DETAIL_CONFIG } from './detail';
import { TRIGGER_LIST_CONFIG } from './list';
import { KillTriggerRunForm } from './trigger-run-action-form';
import { TriggerRunState } from './types';

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
      component: KillTriggerRunForm,
      hierarchy: interpolate(({ data }) =>
        isKillable(data) ? ActionHierarchy.SECONDARY : ActionHierarchy.TERTIARY
      ),
      disabled: [
        {
          condition: interpolate(({ data }) => !isKillable(data)),
          message: 'Only running or paused trigger runs can be killed',
        },
      ],
    },
  ],
};
