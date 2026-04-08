import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { TRIGGER_DETAIL_CONFIG } from './detail';
import { TRIGGER_LIST_CONFIG } from './list';
import {
  KillTriggerRunForm,
  PauseTriggerRunForm,
  ResumeTriggerRunForm,
} from './trigger-run-action-form';
import { TriggerRunState } from './types';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { TriggerRun } from './types';

const isRunning = (record: unknown) =>
  (record as TriggerRun).status?.state === TriggerRunState.RUNNING;

const isPaused = (record: unknown) =>
  (record as TriggerRun).status?.state === TriggerRunState.PAUSED;

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
        isRunning(data) ? ActionHierarchy.SECONDARY : ActionHierarchy.TERTIARY
      ),
      disabled: [
        {
          condition: interpolate(({ data }) => !isRunning(data)),
          message: 'Only running trigger runs can be killed',
        },
      ],
    },
    {
      display: { label: 'Pause', icon: 'pause' },
      component: PauseTriggerRunForm,
      hierarchy: interpolate(({ data }) =>
        isRunning(data) ? ActionHierarchy.PRIMARY : ActionHierarchy.TERTIARY
      ),
      disabled: [
        {
          condition: interpolate(({ data }) => !isRunning(data)),
          message: 'Only running trigger runs can be paused',
        },
      ],
    },
    {
      display: { label: 'Resume', icon: 'playerPlay' },
      component: ResumeTriggerRunForm,
      hierarchy: interpolate(({ data }) =>
        isPaused(data) ? ActionHierarchy.PRIMARY : ActionHierarchy.TERTIARY
      ),
      disabled: [
        {
          condition: interpolate(({ data }) => !isPaused(data)),
          message: 'Only paused trigger runs can be resumed',
        },
      ],
    },
  ],
};
