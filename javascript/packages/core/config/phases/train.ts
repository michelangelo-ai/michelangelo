import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { RUN_ENTITY_CONFIG } from '#core/config/entities/run/run';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';
import { PhaseConfig } from '#core/types/common/studio-types';

export const TRAIN_PHASE: PhaseConfig = {
  id: 'train',
  icon: 'chartLine',
  name: 'Train & Evaluate',
  description: 'Train machine learning models and evaluate their performance',
  docUrl: 'https://example.com/docs/train',
  state: 'active' as const,
  entities: [
    PIPELINE_ENTITY_CONFIG,
    RUN_ENTITY_CONFIG,
    TRIGGER_ENTITY_CONFIG,
    {
      id: 'models',
      name: 'trained models',
      state: 'disabled',
      service: 'model',
      views: [],
    },
    {
      id: 'evaluations',
      name: 'evaluations',
      state: 'disabled',
      service: 'evaluationReport',
      views: [],
    },
    {
      id: 'notebooks',
      name: 'notebooks',
      state: 'disabled',
      service: 'notebook',
      views: [],
    },
  ],
};
