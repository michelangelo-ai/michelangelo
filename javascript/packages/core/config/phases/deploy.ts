import type { PhaseConfig } from '#core/types/common/studio-types';

export const DEPLOY_PHASE: PhaseConfig = {
  id: 'deploy',
  icon: 'deploy',
  name: 'Deploy & Predict',
  description: 'Deploy your models and predict new data',
  docUrl: 'https://example.com/docs/deploy',
  state: 'comingSoon' as const,
  entities: [],
};
