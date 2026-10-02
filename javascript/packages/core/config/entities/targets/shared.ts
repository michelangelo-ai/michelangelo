import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

/**
 * Mirrors the generated proto InferenceServerState enum (inference_server.proto). Colocated here
 * until core has access to the shared generated package — swapping the import path is the only
 * change needed then, since usage sites reference `InferenceServerState.SERVING` etc.
 */
export const InferenceServerState = {
  INVALID: 'INFERENCE_SERVER_STATE_INVALID',
  INITIALIZED: 'INFERENCE_SERVER_STATE_INITIALIZED',
  CREATE_PENDING: 'INFERENCE_SERVER_STATE_CREATE_PENDING',
  SERVING: 'INFERENCE_SERVER_STATE_SERVING',
  FAILED: 'INFERENCE_SERVER_STATE_FAILED',
  DELETE_PENDING: 'INFERENCE_SERVER_STATE_DELETE_PENDING',
  CREATING: 'INFERENCE_SERVER_STATE_CREATING',
  DELETING: 'INFERENCE_SERVER_STATE_DELETING',
  DELETED: 'INFERENCE_SERVER_STATE_DELETED',
} as const;

/**
 * Mirrors the generated proto ConditionStatus enum (inference_server.proto). See
 * InferenceServerState above.
 */
export const ConditionStatus = {
  TRUE: 'CONDITION_STATUS_TRUE',
  FALSE: 'CONDITION_STATUS_FALSE',
  UNKNOWN: 'CONDITION_STATUS_UNKNOWN',
} as const;

export const TENANCY_TYPE = {
  INVALID: 'TENANCY_TYPE_INVALID',
  DEDICATED: 'TENANCY_TYPE_DEDICATED',
  MULTI_TENANT: 'TENANCY_TYPE_MULTI_TENANT',
} as const;

export const BACKEND_TYPE = {
  INVALID: 'BACKEND_TYPE_INVALID',
  TRITON: 'BACKEND_TYPE_TRITON',
  LLM_D: 'BACKEND_TYPE_LLM_D',
  DYNAMO: 'BACKEND_TYPE_DYNAMO',
  TORCHSERVE: 'BACKEND_TYPE_TORCHSERVE',
} as const;

export const BACKEND_TYPE_OPTIONS = [{ id: BACKEND_TYPE.TRITON, label: 'Triton' }];

/** Namespace where platform operators register compute clusters as `Cluster` CRs. */
export const CLUSTER_REGISTRY_NAMESPACE = 'ma-system';

export const CONTAINER_BUILD_TEMPLATE = {
  DEFAULT_TRITON: 'default_triton',
} as const;

export const INFERENCE_SERVER_STATE_CELL: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    [InferenceServerState.INVALID]: 'Invalid',
    [InferenceServerState.INITIALIZED]: 'Initialized',
    [InferenceServerState.CREATE_PENDING]: 'Create pending',
    [InferenceServerState.SERVING]: 'Serving',
    [InferenceServerState.FAILED]: 'Failed',
    [InferenceServerState.DELETE_PENDING]: 'Delete pending',
    [InferenceServerState.CREATING]: 'Initializing',
    [InferenceServerState.DELETING]: 'Deleting',
    [InferenceServerState.DELETED]: 'Deleted',
  },
  stateColorMap: {
    [InferenceServerState.INVALID]: 'gray',
    [InferenceServerState.INITIALIZED]: 'blue',
    [InferenceServerState.CREATE_PENDING]: 'blue',
    [InferenceServerState.SERVING]: 'green',
    [InferenceServerState.FAILED]: 'red',
    [InferenceServerState.DELETE_PENDING]: 'blue',
    [InferenceServerState.CREATING]: 'blue',
    [InferenceServerState.DELETING]: 'blue',
    [InferenceServerState.DELETED]: 'gray',
  },
};
