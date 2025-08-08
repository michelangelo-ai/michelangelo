import { PhaseConfig } from '#core/types/common/studio-types';

export const TRAIN_PHASE: PhaseConfig = {
  id: 'train',
  icon: 'chartLine',
  name: 'Train & Evaluate',
  description: 'Train machine learning models and evaluate their performance',
  docUrl: 'https://example.com/docs/train',
  state: 'active' as const,
  entities: [
    { id: 'pipelines', name: 'pipelines', state: 'active' },
    { id: 'runs', name: 'runs', state: 'active' },
    { id: 'models', name: 'trained models', state: 'disabled' },
    { id: 'evaluations', name: 'evaluations', state: 'disabled' },
    { id: 'notebooks', name: 'notebooks', state: 'disabled' },
  ],
};
