import { useQuery } from '@tanstack/react-query';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { QueryOptions, QueryResult } from '#core/types/query-types';

export const useStudioQuery = <TData>(args: {
  queryName: string;
  serviceOptions: Record<string, unknown>;
  clientOptions?: QueryOptions;
}): QueryResult<TData> => {
  const { queryName, serviceOptions, clientOptions } = args;
  const { projectId } = useStudioParams('base');
  const { request } = useServiceProvider();

  // A CR's namespace is the projectId, but the serviceOptions may provide a different namespace
  // to find the CR in an alternate namespace. e.g., "default" namespace for a new Project.
  const namespace = serviceOptions?.namespace ?? projectId;

  return useQuery<TData, Error, TData, [string, Record<string, unknown>]>({
    queryKey: [queryName, { ...serviceOptions, namespace }],
    queryFn: () => request(queryName, { ...serviceOptions, namespace }) as Promise<TData>,
    ...clientOptions,
  });
};
