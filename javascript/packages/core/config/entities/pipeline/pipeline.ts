import { CreatePipelineRunDialog } from './create-pipeline-run-dialog';
import { PIPELINE_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'Pipelines',
  service: 'pipeline',
  state: 'active',
  views: [PIPELINE_LIST_CONFIG],
  actions: CreatePipelineRunDialog,
};
