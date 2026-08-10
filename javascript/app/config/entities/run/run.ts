import { RUN_DETAIL_CONFIG } from './detail';
import { RUN_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '@michelangelo-ai/core';

export const RUN_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'runs',
  name: 'Pipeline Runs',
  service: 'pipelineRun',
  state: 'active',
  views: [RUN_LIST_CONFIG, RUN_DETAIL_CONFIG],
};
