import { PIPELINE_DETAIL_CONFIG } from './detail';
import { PIPELINE_LIST_CONFIG } from './list';
import { PIPELINE_ACTIONS } from './shared';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'pipelines',
  service: 'pipeline',
  state: 'active',
  revisioned: true,
  views: [PIPELINE_LIST_CONFIG, PIPELINE_DETAIL_CONFIG],
  actions: PIPELINE_ACTIONS,
};
