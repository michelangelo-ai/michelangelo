import { DEPLOYMENT_ENTITY_CONFIG } from '../entities/deployment/deployment';
import { TARGET_ENTITY_CONFIG } from '../entities/targets/target';

import type { PhaseConfig } from '@michelangelo-ai/core';

export const DEPLOY_PHASE: PhaseConfig = {
  id: 'deploy',
  icon: 'deploy',
  name: 'Deploy & Predict',
  description: 'Deploy your models and predict new data',
  state: 'comingSoon' as const,
  entities: [TARGET_ENTITY_CONFIG, DEPLOYMENT_ENTITY_CONFIG],
};
