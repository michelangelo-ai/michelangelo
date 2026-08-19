import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

/**
 * Mirrors the generated proto PipelineState enum (pipeline.proto). Colocated here until core
 * has access to the shared generated package — swapping the import path is the only change
 * needed then, since usage sites reference `PipelineState.READY` etc.
 */
export const PipelineState = {
  INVALID: 'PIPELINE_STATE_INVALID',
  CREATED: 'PIPELINE_STATE_CREATED',
  BUILDING: 'PIPELINE_STATE_BUILDING',
  READY: 'PIPELINE_STATE_READY',
  ERROR: 'PIPELINE_STATE_ERROR',
} as const;

/**
 * Mirrors the generated proto PipelineType enum (pipeline.proto). See PipelineState above.
 */
export const PipelineType = {
  INVALID: 'PIPELINE_TYPE_INVALID',
  TRAIN: 'PIPELINE_TYPE_TRAIN',
  EVAL: 'PIPELINE_TYPE_EVAL',
  PERF_EVAL: 'PIPELINE_TYPE_PERF_EVAL',
  EXPERIMENT: 'PIPELINE_TYPE_EXPERIMENT',
  RETRAIN: 'PIPELINE_TYPE_RETRAIN',
  PREDICTION: 'PIPELINE_TYPE_PREDICTION',
  PERFORMANCE_MONITORING: 'PIPELINE_TYPE_PERFORMANCE_MONITORING',
  BASIS_FEATURE: 'PIPELINE_TYPE_BASIS_FEATURE',
  DATA_PREP: 'PIPELINE_TYPE_DATA_PREP',
  ONLINE_OFFLINE_FEATURE_CONSISTENCY: 'PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY',
  FEATURE_GROUP_COMPUTE: 'PIPELINE_TYPE_FEATURE_GROUP_COMPUTE',
  ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION:
    'PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION',
  POST_PROCESSING: 'PIPELINE_TYPE_POST_PROCESSING',
  OPTIMIZATION: 'PIPELINE_TYPE_OPTIMIZATION',
  SCORER: 'PIPELINE_TYPE_SCORER',
} as const;

export const PIPELINE_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    [PipelineState.INVALID]: 'Invalid',
    [PipelineState.CREATED]: 'Created',
    [PipelineState.BUILDING]: 'Building',
    [PipelineState.READY]: 'Ready',
    [PipelineState.ERROR]: 'Error',
  },
  stateColorMap: {
    [PipelineState.INVALID]: 'red',
    [PipelineState.CREATED]: 'green',
    [PipelineState.BUILDING]: 'yellow',
    [PipelineState.READY]: 'green',
    [PipelineState.ERROR]: 'red',
  },
};

export const PIPELINE_TYPE_CELL: Cell = {
  id: 'spec.type',
  label: 'Type',
  type: CellType.TYPE,
  typeTextMap: {
    [PipelineType.INVALID]: 'Invalid',
    [PipelineType.TRAIN]: 'Train',
    [PipelineType.EVAL]: 'Evaluation',
    [PipelineType.PERF_EVAL]: 'Performance Evaluation',
    [PipelineType.EXPERIMENT]: 'Experiment',
    [PipelineType.RETRAIN]: 'Retrain',
    [PipelineType.PREDICTION]: 'Prediction',
    [PipelineType.PERFORMANCE_MONITORING]: 'Performance Monitoring',
    [PipelineType.BASIS_FEATURE]: 'Basis Feature',
    [PipelineType.DATA_PREP]: 'Data Prep',
    [PipelineType.ONLINE_OFFLINE_FEATURE_CONSISTENCY]: 'Online Offline Feature Consistency',
    [PipelineType.FEATURE_GROUP_COMPUTE]: 'Feature Group Compute',
    [PipelineType.ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION]:
      'Online Offline Feature Consistency Orchestration',
    [PipelineType.POST_PROCESSING]: 'Post Processing',
    [PipelineType.OPTIMIZATION]: 'Optimization',
    [PipelineType.SCORER]: 'Scorer',
  },
};
