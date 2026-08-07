import { DEPLOYMENT_ENTITY_CONFIG } from '#core/config/entities/deployment/deployment';
import { TARGET_ENTITY_CONFIG } from '#core/config/entities/targets/target';

import type { PhaseConfig } from '#core/types/common/studio-types';

export const DEPLOY_PHASE: PhaseConfig = {
  id: 'deploy',
  icon: 'deploy',
  name: 'Deploy & Predict',
  description: 'Deploy your models and predict new data',
  state: 'active' as const,
  entities: [TARGET_ENTITY_CONFIG, DEPLOYMENT_ENTITY_CONFIG],
};
