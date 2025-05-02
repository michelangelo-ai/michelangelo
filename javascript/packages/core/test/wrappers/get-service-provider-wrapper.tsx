import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi } from 'vitest';

import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { ServiceContextType } from '#core/providers/service-provider/types';
import { WrapperComponentProps } from './types';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
    },
  },
});

/**
 * Creates a React wrapper for testing components that use service features.
 * This wrapper is essential for testing components that use service hooks
 * like useStudioQuery, useStudioMutation, etc.
 *
 * @param serviceProvider - The service provider to use for the service context
 * @returns A wrapper component that provides service context to its children
 *
 * @example
 * ```tsx
 * // Simple usage with a specific route
 * const mockRequest = vi.fn();
 * const wrapper = getServiceProviderWrapper({ request: mockRequest });
 * render(<MyComponent />, { wrapper });
 *
 * expect(mockRequest).toHaveBeenCalledWith('requestId', { queryKey: ['requestId'] });
 * ```
 */
export function getServiceProviderWrapper(serviceProvider: Partial<ServiceContextType>) {
  const mockRequest = vi.fn();
  const base = {
    request: mockRequest,
  };

  return function ServiceProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <QueryClientProvider client={queryClient}>
        <ServiceProvider {...base} {...serviceProvider}>
          {children}
        </ServiceProvider>
      </QueryClientProvider>
    );
  };
}
