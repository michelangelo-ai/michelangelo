import { useStudioQuery as _useStudioQuery } from '#core/hooks/use-studio-query';

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
