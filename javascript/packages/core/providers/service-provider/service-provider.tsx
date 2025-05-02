import { ServiceContext } from './service-context';

import type { ServiceContextType } from './types';

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
export const ServiceProvider = ({
  children,
  ...serviceContext
}: { children: React.ReactNode } & ServiceContextType) => {
  return <ServiceContext.Provider value={serviceContext}>{children}</ServiceContext.Provider>;
};
