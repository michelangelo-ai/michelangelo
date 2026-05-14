import { useStudioMutation as _useStudioMutation } from '#core/hooks/use-studio-mutation';
import { useStudioQuery as _useStudioQuery } from '#core/hooks/use-studio-query';

import type { ApplicationError } from '#core/types/error-types';

/**
 * Configuration for a query that can be used by {@link _useStudioQuery}
 */
export interface QueryConfig {
  /** Lowercase endpoint of the service to query, e.g. 'get', 'list' */
  endpoint: string;

  /** camelCase name of the service to query, e.g. 'pipelineRun' */
  service: string;

  /** Options to pass to the service, e.g. 'namespace', 'name' */
  serviceOptions: Record<string, unknown>;

  /** Options to pass to the query, e.g. 'enabled' */
  clientOptions?: QueryOptions;
}

/**
 * Options that can be passed to query hooks.
 */
export type QueryOptions = {
  /** Whether the query should be enabled */
  enabled?: boolean;
};

/**
 * Configuration for a mutation that can be used by {@link _useStudioMutation}.
 */
export interface MutationConfig {
  /** Name of the RPC mutation handler to call, e.g. 'CreatePipelineRun' */
  mutationName: string;

  /** Options to pass to the mutation, e.g. 'onSuccess', 'onError' */
  clientOptions?: MutationOptions;
}

/**
 * Options that can be passed to mutation hooks.
 *
 * Callbacks expose simplified signatures — callers don't see React Query's
 * full `(data, variables, context?)` shape.
 */
export type MutationOptions = {
  /** Called with the mutation response when the request succeeds */
  onSuccess?: (data: unknown) => void;

  /** Called with the normalized error when the request fails */
  onError?: (error: ApplicationError) => void;
};
