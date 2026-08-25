import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

export const TARGET_TYPE_LABELS: Record<string, string> = {
  TARGET_TYPE_INFERENCE_SERVER: 'Online',
  TARGET_TYPE_OFFLINE: 'Offline',
  TARGET_TYPE_MOBILE: 'Mobile',
  TARGET_TYPE_SELF_HOSTED: 'Self-hosted',
};

/**
 * Mirrors the generated proto DeploymentStage enum (deployment.proto). Colocated here until core
 * has access to the shared generated package — swapping the import path is the only change
 * needed then, since usage sites reference `DeploymentStage.ROLLOUT_FAILED` etc.
 */
export const DeploymentStage = {
  INVALID: 'DEPLOYMENT_STAGE_INVALID',
  VALIDATION: 'DEPLOYMENT_STAGE_VALIDATION',
  PLACEMENT: 'DEPLOYMENT_STAGE_PLACEMENT',
  RESOURCE_ACQUISITION: 'DEPLOYMENT_STAGE_RESOURCE_ACQUISITION',
  ROLLOUT_COMPLETE: 'DEPLOYMENT_STAGE_ROLLOUT_COMPLETE',
  ROLLOUT_FAILED: 'DEPLOYMENT_STAGE_ROLLOUT_FAILED',
  ROLLBACK_IN_PROGRESS: 'DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS',
  ROLLBACK_COMPLETE: 'DEPLOYMENT_STAGE_ROLLBACK_COMPLETE',
  ROLLBACK_FAILED: 'DEPLOYMENT_STAGE_ROLLBACK_FAILED',
  CLEAN_UP_IN_PROGRESS: 'DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS',
  CLEAN_UP_COMPLETE: 'DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE',
  CLEAN_UP_FAILED: 'DEPLOYMENT_STAGE_CLEAN_UP_FAILED',
} as const;

/**
 * Mirrors the generated proto DeploymentState enum (deployment.proto). See DeploymentStage above.
 */
export const DeploymentState = {
  INVALID: 'DEPLOYMENT_STATE_INVALID',
  INITIALIZING: 'DEPLOYMENT_STATE_INITIALIZING',
  HEALTHY: 'DEPLOYMENT_STATE_HEALTHY',
  UNHEALTHY: 'DEPLOYMENT_STATE_UNHEALTHY',
  EMPTY: 'DEPLOYMENT_STATE_EMPTY',
} as const;

/**
 * Mirrors the generated proto DeploymentConditionStatus enum (deployment.proto). See
 * DeploymentStage above.
 */
export const DeploymentConditionStatus = {
  TRUE: 'CONDITION_STATUS_TRUE',
  FALSE: 'CONDITION_STATUS_FALSE',
  UNKNOWN: 'CONDITION_STATUS_UNKNOWN',
} as const;

export const DEPLOYMENT_STAGE_CELL: Cell = {
  id: 'status.stage',
  label: 'Stage',
  type: CellType.TYPE,
  typeTextMap: {
    [DeploymentStage.INVALID]: 'Invalid',
    [DeploymentStage.VALIDATION]: 'Validation',
    [DeploymentStage.PLACEMENT]: 'Placement',
    [DeploymentStage.RESOURCE_ACQUISITION]: 'Resource acquisition',
    [DeploymentStage.ROLLOUT_COMPLETE]: 'Rollout complete',
    [DeploymentStage.ROLLOUT_FAILED]: 'Rollout failed',
    [DeploymentStage.ROLLBACK_IN_PROGRESS]: 'Rollback in progress',
    [DeploymentStage.ROLLBACK_COMPLETE]: 'Rollback complete',
    [DeploymentStage.ROLLBACK_FAILED]: 'Rollback failed',
    [DeploymentStage.CLEAN_UP_IN_PROGRESS]: 'Clean up in progress',
    [DeploymentStage.CLEAN_UP_COMPLETE]: 'Clean up complete',
    [DeploymentStage.CLEAN_UP_FAILED]: 'Clean up failed',
  },
};

export const DEPLOYMENT_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    [DeploymentState.INVALID]: 'Invalid',
    [DeploymentState.INITIALIZING]: 'Initializing',
    [DeploymentState.HEALTHY]: 'Healthy',
    [DeploymentState.UNHEALTHY]: 'Unhealthy',
    [DeploymentState.EMPTY]: 'Empty',
  },
  stateColorMap: {
    [DeploymentState.INVALID]: 'gray',
    [DeploymentState.INITIALIZING]: 'blue',
    [DeploymentState.HEALTHY]: 'green',
    [DeploymentState.UNHEALTHY]: 'red',
    [DeploymentState.EMPTY]: 'gray',
  },
};
