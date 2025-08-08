import { RUN_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const RUN_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'runs',
  name: 'Pipeline Runs',
  service: 'pipelineRun',
  state: 'active',
  views: [RUN_LIST_CONFIG],
};
