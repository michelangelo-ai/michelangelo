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
      display: { label: 'Kill' },
      component: KillTriggerRunForm,
      disabled: [
        { condition: (r) => !isRunning(r), message: 'Only running trigger runs can be killed' },
      ],
    },
    {
      display: { label: 'Pause' },
      component: PauseTriggerRunForm,
      disabled: [
        { condition: (r) => !isRunning(r), message: 'Only running trigger runs can be paused' },
      ],
    },
    {
      display: { label: 'Resume' },
      component: ResumeTriggerRunForm,
      disabled: [
        { condition: (r) => !isPaused(r), message: 'Only paused trigger runs can be resumed' },
      ],
    },
  ],
};
