import { ActionHierarchy } from '@michelangelo-ai/core';

import { CreatePipelineRunForm } from './create-pipeline-run-form';
import { PIPELINE_DETAIL_CONFIG } from './detail';
import { PIPELINE_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '@michelangelo-ai/core';

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'Pipelines',
  service: 'pipeline',
  state: 'active',
  views: [PIPELINE_LIST_CONFIG, PIPELINE_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Run', icon: 'playerPlay' },
      hierarchy: ActionHierarchy.PRIMARY,
      modal: { type: 'custom', component: CreatePipelineRunForm },
    },
  ],
};
