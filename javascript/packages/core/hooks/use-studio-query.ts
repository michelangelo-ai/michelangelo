import { useQuery } from '@tanstack/react-query';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { UseQueryResult } from '@tanstack/react-query';
import type { ApplicationError } from '#core/types/error-types';
import type { QueryOptions } from '#core/types/query-types';

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
        throw normalizeError(error)!;
      }
    },
    ...clientOptions,
  });
};
