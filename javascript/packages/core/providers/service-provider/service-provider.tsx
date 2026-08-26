import { useMemo } from 'react';

import { ServiceContext } from './service-context';
import { withOwnershipEnrichment } from './with-ownership-enrichment';

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
  request,
  resolvers,
}: { children: React.ReactNode } & ServiceContextType) => {
  const value = useMemo(
    () => ({ request: withOwnershipEnrichment(request, resolvers?.team), resolvers }),
    [request, resolvers]
  );

  return <ServiceContext.Provider value={value}>{children}</ServiceContext.Provider>;
};
