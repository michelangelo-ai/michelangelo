import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { CreatePipelineRunForm } from './create-pipeline-run-form';
import { PIPELINE_DETAIL_CONFIG } from './detail';
import { PIPELINE_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

// Placeholder: all pipeline actions are disabled until real implementations are wired up.
const PIPELINE_NOT_AVAILABLE = [
  { condition: interpolate(() => true), message: 'Pipeline is not available' },
];

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'Pipelines',
  service: 'pipeline',
  state: 'active',
  views: [PIPELINE_LIST_CONFIG, PIPELINE_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Run', icon: 'playerPlay' },
      disabled: PIPELINE_NOT_AVAILABLE,
      component: CreatePipelineRunForm,
    },
    {
      display: { label: 'Edit', icon: 'pencil' },
      disabled: PIPELINE_NOT_AVAILABLE,
      component: CreatePipelineRunForm,
    },
    {
      display: { label: 'Delete', icon: 'trash' },
      disabled: PIPELINE_NOT_AVAILABLE,
      component: CreatePipelineRunForm,
      hierarchy: ActionHierarchy.PRIMARY,
    },
  ],
};
