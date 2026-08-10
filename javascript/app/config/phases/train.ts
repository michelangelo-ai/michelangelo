import { MODEL_ENTITY_CONFIG } from '../entities/model/model';
import { PIPELINE_ENTITY_CONFIG } from '../entities/pipeline/pipeline';
import { PIPELINE_RUN_ENTITY_CONFIG } from '../entities/pipeline-run/pipeline-run';
import { TRIGGER_ENTITY_CONFIG } from '../entities/trigger/trigger';

import type { PhaseConfig } from '@michelangelo-ai/core';

export const TRAIN_PHASE: PhaseConfig = {
  id: 'train',
  icon: 'chartLine',
  name: 'Train & Evaluate',
  description: 'Train machine learning models and evaluate their performance',
  docUrl:
    'https://michelangelo-ai.org/docs/user-guides/train-and-deploy-models/train-and-register-a-model',
  state: 'active' as const,
  entities: [
    PIPELINE_ENTITY_CONFIG,
    PIPELINE_RUN_ENTITY_CONFIG,
    TRIGGER_ENTITY_CONFIG,
    MODEL_ENTITY_CONFIG,
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
