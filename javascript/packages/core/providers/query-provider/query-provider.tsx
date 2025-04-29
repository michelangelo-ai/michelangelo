import { QueryContext } from './query-context';

import type { QueryContextType } from './types';

/**
 * @description
 * Provides the query context to the application. This configuration provided to the
 * {@link QueryProvider} should be able to connect to the Michelangelo API yarpc server.
 *
 * @remarks
 * Leverages {@link QueryContext} to provide the query context to the application.
 *
 * @example
 * ```tsx
 * <QueryProvider
 *   useQuery={useQuery}
 * >
 *   <App />
 * </QueryProvider>
 * ```
 */
export const QueryProvider = ({
  children,
  ...queryContext
}: { children: React.ReactNode } & QueryContextType) => {
  return <QueryContext.Provider value={queryContext}>{children}</QueryContext.Provider>;
};
