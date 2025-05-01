import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useQueryProvider } from '#core/providers/query-provider/use-query-provider';

import type { QueryOptions, QueryResult } from '#core/types/query-types';

export const useStudioQuery = <TData>(args: {
  queryName: string;
  serviceOptions: Record<string, unknown>;
  clientOptions?: QueryOptions;
}): QueryResult<TData> => {
  const { queryName, serviceOptions, clientOptions } = args;
  const { projectId } = useStudioParams('base');
  const { useQuery } = useQueryProvider();

  return useQuery<TData>(
    queryName,
    { ...serviceOptions, namespace: serviceOptions?.namespace ?? projectId },
    clientOptions
  );
};
