import { TRIGGER_DETAIL_CONFIG } from './detail';
import { TRIGGER_LIST_CONFIG } from './list';
import { TriggerRunActions } from './trigger-run-actions';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const TRIGGER_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'triggers',
  name: 'Triggers',
  service: 'triggerRun',
  state: 'active',
  views: [TRIGGER_LIST_CONFIG, TRIGGER_DETAIL_CONFIG],
  actions: TriggerRunActions,
};
