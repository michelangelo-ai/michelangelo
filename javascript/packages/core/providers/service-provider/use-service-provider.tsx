import { useContext } from 'react';

import { ServiceContext } from './service-context';

/**
 * Accesses the service context to make RPC requests to the Michelangelo API.
 *
 * This hook must be used within a ServiceProvider component. It provides access
 * to the request function for making API calls.
 *
 * @returns Service context containing the request function. Note: The returned
 *   `request` function will throw if called outside of a ServiceProvider.
 *
 * @example
 * ```typescript
 * function MyComponent() {
 *   const { request } = useServiceProvider();
 *
 *   const fetchPipeline = async () => {
 *     const pipeline = await request('GetPipeline', {
 *       name: 'my-pipeline',
 *       namespace: 'my-project'
 *     });
 *     return pipeline;
 *   };
 *
 *   // Use with useStudioQuery for automatic caching/error handling
 *   // (preferred over direct usage)
 * }
 * ```
 */
export const useServiceProvider = () => {
  return useContext(ServiceContext);
};
