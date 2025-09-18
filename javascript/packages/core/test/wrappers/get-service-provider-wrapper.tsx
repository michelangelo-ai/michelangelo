import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { isEqual } from 'lodash';
import { vi } from 'vitest';

import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { ServiceContextType } from '#core/providers/service-provider/types';
import { WrapperComponentProps } from './types';

/**
 * Creates a React wrapper for testing components that use service features.
 * This wrapper is essential for testing components that use service hooks
 * like useStudioQuery, useStudioMutation, etc.
 *
 * Each wrapper gets a fresh QueryClient instance to ensure test isolation.
 *
 * @remarks
 * Use {@link createQueryMockRouter} for complex mocking scenarios.
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
 *
 * // Usage with multiple requests
 * const mockRequest = createQueryMockRouter({
 *   'GetPipelineRun:{"namespace":"project-name","name":"pipeline-name"}': { pipelineRun: { name: 'test' } },
 *   'ListPipelineRun': { pipelineRunList: { items: [] } },
 * });
 * const wrapper = getServiceProviderWrapper({ request: mockRequest });
 * render(<MyComponent />, { wrapper });
 *
 * expect(mockRequest).toHaveBeenCalledWith('GetPipelineRun', { queryKey: ['GetPipelineRun'] });
 * expect(mockRequest).toHaveBeenCalledWith('ListPipelineRun', { queryKey: ['ListPipelineRun'] });
 * ```
 */
export function getServiceProviderWrapper(serviceProvider: Partial<ServiceContextType>) {
  const mockRequest = vi.fn();
  const base = {
    request: mockRequest,
  };

  // Create a fresh QueryClient for each test to ensure isolation
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

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

/**
 * Creates a query-aware mock request function that responds based on queryName and serviceOptions.
 *
 * @param responses - Map of queryName or "queryName:serviceOptions" to response data or Error
 * @returns Mock function that routes requests based on queryName and optionally serviceOptions
 *
 * @example
 * ```tsx
 * const mockRequest = createQueryMockRouter({
 *   'GetPipelineRun': { pipelineRun: { name: 'test' } },
 *   'ListPipelineRun': { pipelineRunList: { items: [] } },
 *   'ListPipeline:{"namespace":"project","filter":"status=SUCCESS"}': { // Exact serviceOptions match
 *     pipelineList: { items: [{ name: 'filtered' }] }
 *   },
 * });
 *
 * const wrapper = getServiceProviderWrapper({ request: mockRequest });
 * ```
 */
export function createQueryMockRouter(
  responses: Record<string, object | Error>
): ServiceContextType['request'] {
  return vi.fn((queryName: string, args: object) => {
    if (args) {
      for (const [responseKey, response] of Object.entries(responses)) {
        if (isEqual(parseArgsFromKey(responseKey), { queryName, args })) {
          return response instanceof Error ? Promise.reject(response) : Promise.resolve(response);
        }
      }
    }

    const response = responses[queryName];
    if (response === undefined) {
      const argsStr = args ? JSON.stringify(args) : 'undefined';
      return Promise.reject(new Error(`Unexpected query: ${queryName} with args: ${argsStr}`));
    }

    return response instanceof Error ? Promise.reject(response) : Promise.resolve(response);
  });
}

/** Parse query args from a response key like "QueryName:{"key":"value"}" */
function parseArgsFromKey(key: string): { queryName: string; args: object } | null {
  const colonIndex = key.indexOf(':');
  if (colonIndex === -1) return null;

  const queryName = key.slice(0, colonIndex);
  const argsStr = key.slice(colonIndex + 1);

  try {
    const args = JSON.parse(argsStr) as object;
    return { queryName, args };
  } catch {
    return null;
  }
}
