import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { RUN_ENTITY_CONFIG } from '#core/config/entities/run/run';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';

import type { PhaseConfig } from '#core/types/common/studio-types';

export const EXPERIMENT_PRODUCTIONIZE_PHASE: PhaseConfig = {
  id: 'retrain',
  icon: 'rocket',
  name: 'Experiment & Productionize',
  description: 'Experiment with pipelines and productionize your machine learning workflows',
  state: 'active' as const,
  entities: [PIPELINE_ENTITY_CONFIG, RUN_ENTITY_CONFIG, TRIGGER_ENTITY_CONFIG],
};
