import { ServiceContext } from './service-context';

import type { ServiceContextType } from './types';

/**
 * @description
 * Provides the ability to request data from or send data to a server.
 *
 * @remarks
 * Internally, the `ServiceProvider` uses Tanstack Query QueryClient to manage data fetching,
 * so this provider also provides Tanstack Query's QueryClientProvider.
 *
 * @example
 * ```tsx
 * <ServiceProvider request={request}>
 *   <MyComponent />
 * </ServiceProvider>
 * ```
 */
export const ServiceProvider = ({
  children,
  ...serviceContext
}: { children: React.ReactNode } & ServiceContextType) => {
  return <ServiceContext.Provider value={serviceContext}>{children}</ServiceContext.Provider>;
};
