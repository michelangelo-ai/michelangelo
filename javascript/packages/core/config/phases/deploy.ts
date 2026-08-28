import { DEPLOYMENT_ENTITY_CONFIG } from '#core/config/entities/deployment/deployment';
import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { RUN_ENTITY_CONFIG } from '#core/config/entities/run/run';
import { TARGET_ENTITY_CONFIG } from '#core/config/entities/targets/target';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';

import type { PhaseConfig } from '#core/types/common/studio-types';

export const DEPLOY_PHASE: PhaseConfig = {
  id: 'deploy',
  icon: 'deploy',
  name: 'Deploy & Predict',
  description: 'Deploy your models and predict new data',
  state: 'comingSoon' as const,
  pipelineTypes: ['PIPELINE_TYPE_PREDICTION', 'PIPELINE_TYPE_SCORER'],
  entities: [
    PIPELINE_ENTITY_CONFIG,
    TRIGGER_ENTITY_CONFIG,
    RUN_ENTITY_CONFIG,
    TARGET_ENTITY_CONFIG,
    DEPLOYMENT_ENTITY_CONFIG,
  ],
};
