import { PhaseConfig } from '#core/types/common/studio-types';

export const DATA_PHASE: PhaseConfig = {
  id: 'data',
  icon: 'database',
  name: 'Prepare & Analyze Data',
  description: 'Create data pipelines and analyze your datasets',
  docUrl: 'https://example.com/docs/data',
  state: 'disabled' as const,
  entities: [
    { id: 'pipelines', name: 'pipelines', state: 'active' },
    { id: 'datasources', name: 'data sources', state: 'active' },
    { id: 'runs', name: 'runs', state: 'active' },
  ],
};
