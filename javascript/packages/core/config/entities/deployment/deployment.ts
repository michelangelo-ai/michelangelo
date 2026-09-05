import { CreateDeploymentForm } from './create-deployment-form';
import { DEPLOYMENT_DETAIL_CONFIG } from './detail';
import { DEPLOYMENT_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const DEPLOYMENT_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'deployments',
  name: 'deployments',
  service: 'deployment',
  state: 'active',
  views: [DEPLOYMENT_LIST_CONFIG, DEPLOYMENT_DETAIL_CONFIG],
  createAction: {
    display: { label: 'Create deployment', icon: 'plus' },
    component: CreateDeploymentForm,
  },
};
