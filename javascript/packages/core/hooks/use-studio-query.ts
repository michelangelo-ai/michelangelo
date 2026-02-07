import { useQuery } from '@tanstack/react-query';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { UseQueryResult } from '@tanstack/react-query';
import type { ApplicationError } from '#core/types/error-types';
import type { QueryOptions } from '#core/types/query-types';

/**
 * Executes a query to fetch data from the Michelangelo API with automatic error normalization,
 * interpolation resolution, and namespace management.
 *
 * This hook wraps Tanstack Query's useQuery with Michelangelo-specific functionality:
 * - Resolves interpolated values in serviceOptions before making the request
 * - Automatically uses projectId as namespace unless overridden
 * - Normalizes RPC errors to ApplicationError format
 * - Integrates with the service provider's request function
 *
 * @template TData - The expected type of data returned by the query
 *
 * @param args.queryName - The name of the RPC method to call (e.g., 'ListProject', 'GetPipeline')
 * @param args.serviceOptions - Options passed to the RPC handler. Can include interpolated values
 *   that will be resolved before the request is made. May include a `namespace` to override
 *   the default projectId namespace.
 * @param args.clientOptions - Additional Tanstack Query options (enabled, refetchInterval, etc.)
 *
 * @returns Tanstack Query result with typed data and normalized errors
 *
 * @example
 * ```typescript
 * // Basic usage - fetches pipeline in current project
 * const { data, isLoading, error } = useStudioQuery({
 *   queryName: 'GetPipeline',
 *   serviceOptions: { name: 'my-pipeline' }
 * });
 *
 * // With interpolation - name resolved at runtime
 * const { data } = useStudioQuery({
 *   queryName: 'GetPipeline',
 *   serviceOptions: { name: interpolate('${pipeline.name}') }
 * });
 *
 * // Custom namespace - fetch from different project/namespace
 * const { data } = useStudioQuery({
 *   queryName: 'GetPipeline',
 *   serviceOptions: {
 *     name: 'my-pipeline',
 *     namespace: 'other-project'
 *   }
 * });
 *
 * // With client options - disable until user action
 * const { data, refetch } = useStudioQuery({
 *   queryName: 'ListPipelineRuns',
 *   serviceOptions: { pipelineName: 'training-pipeline' },
 *   clientOptions: { enabled: false }
 * });
 * ```
 */
export const useStudioQuery = <TData>(args: {
  queryName: string;
  serviceOptions: Record<string, unknown>;
  clientOptions?: QueryOptions;
}): UseQueryResult<TData, ApplicationError> => {
  const { queryName, clientOptions } = args;
  const { projectId } = useStudioParams('base');
  const { request } = useServiceProvider();
  const normalizeError = useErrorNormalizer();
  const resolver = useInterpolationResolver();

  const serviceOptions = resolver(args.serviceOptions);
  // A CR's namespace is the projectId, but the serviceOptions may provide a different namespace
  // to find the CR in an alternate namespace. e.g., "default" namespace for a new Project.
  const namespace = serviceOptions?.namespace ?? projectId;

  return useQuery<TData, ApplicationError, TData, [string, Record<string, unknown>]>({
    queryKey: [queryName, { ...serviceOptions, namespace }],
    queryFn: async () => {
      try {
        return (await request(queryName, { ...serviceOptions, namespace })) as Promise<TData>;
      } catch (error) {
        console.log('error', error);
        throw normalizeError(error)!;
      }
    },
    ...clientOptions,
  });
};
