import { PhaseConfig } from '#core/types/common/studio-types';
import { PIPELINE_ENTITY_CONFIG } from '../entities/pipeline/pipeline';
import { RUN_ENTITY_CONFIG } from '../entities/run/run';

export const DATA_PHASE: PhaseConfig = {
  id: 'data',
  icon: 'database',
  name: 'Prepare & Analyze Data',
  description: 'Create data pipelines and analyze your datasets',
  docUrl: 'https://example.com/docs/data',
  state: 'disabled' as const,
  entities: [
    { ...PIPELINE_ENTITY_CONFIG, state: 'disabled' },
    { ...RUN_ENTITY_CONFIG, state: 'disabled' },
    {
      id: 'datasources',
      name: 'data sources',
      state: 'disabled',
      service: 'datasource',
      views: [],
    },
  ],
};
