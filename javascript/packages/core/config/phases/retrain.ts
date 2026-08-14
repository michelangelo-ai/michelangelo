import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { RUN_ENTITY_CONFIG } from '#core/config/entities/run/run';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';

import type { PhaseConfig } from '#core/types/common/studio-types';

export const RETRAIN_PHASE: PhaseConfig = {
  id: 'retrain',
  icon: 'lightbulb',
  name: 'Experiment & Productionize',
  description: 'Experiment with pipelines and productionize your machine learning workflows',
  state: 'active' as const,
  entities: [PIPELINE_ENTITY_CONFIG, RUN_ENTITY_CONFIG, TRIGGER_ENTITY_CONFIG],
};
