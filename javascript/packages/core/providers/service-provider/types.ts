import type { QueryOptions, QueryResult } from '#core/types/query-types';

/**
 * @description
 * The hooks provided to the application to connect to the services injected
 * into the application.
 *
 * @remarks
 * Since the available queryIds are injected into the application, the parameters and
 * return types are unknown.
 */
export type ServiceContextType = {
  useQuery: <TData>(queryId: string, args: unknown, options?: QueryOptions) => QueryResult<TData>;
};
