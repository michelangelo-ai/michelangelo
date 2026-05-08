import { TARGET_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const TARGET_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'targets',
  name: 'Targets',
  service: 'inferenceServer',
  state: 'active',
  views: [TARGET_LIST_CONFIG],
};
