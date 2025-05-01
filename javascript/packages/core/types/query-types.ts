/**
 * Options that can be passed to query hooks.
 */
export type QueryOptions = {
  /** Whether the query should be enabled */
  enabled?: boolean;
};

/**
 * Standard result structure returned by query hooks.
 */
export type QueryResult<TData = unknown> = {
  /** The data returned by the query */
  data: TData | undefined;
  /** Any error that occurred during the query */
  error: Error | null;
  /** Whether the query is currently loading */
  isLoading: boolean;
};
