import type { PhaseConfig } from '#core/types/common/studio-types';

export const MONITOR_DEBUG_PHASE: PhaseConfig = {
  id: 'monitor-debug',
  icon: 'monitor',
  name: 'Monitor & Debug',
  description: 'Monitor model performance and debug issues in production',
  state: 'comingSoon' as const,
  entities: [],
};
