import type { PhaseConfig } from '#core/types/common/studio-types';

export const MONITOR_DEBUG_PHASE: PhaseConfig = {
  id: 'monitor',
  icon: 'monitor',
  name: 'Monitor & Debug',
  description: 'Monitor model performance and debug issues in production',
  state: 'comingSoon' as const,
  pipelineTypes: [
    'PIPELINE_TYPE_PERF_EVAL',
    'PIPELINE_TYPE_PERFORMANCE_MONITORING',
    'PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY',
    'PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION',
  ],
  entities: [],
};
