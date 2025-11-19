import { MODEL_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const MODEL_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'models',
  name: 'trained models',
  service: 'model',
  state: 'active',
  views: [MODEL_LIST_CONFIG],
};