import { MemoryRouter, Route, Routes } from 'react-router-dom';

import { WrapperComponentProps } from './types';

/**
 * Creates a React Router wrapper for testing components that use routing features.
 * This wrapper is essential for testing components that use react-router hooks
 * like useParams, useStudioParams, etc. Without this wrapper, tests will fail
 * with errors like "Cannot read properties of undefined (reading 'match')".
 *
 * @param options - Configuration options for the router
 * @param options.location - Initial URL path to render (defaults to '/')
 * @returns A wrapper component that provides routing context to its children
 *
 * @example
 * ```tsx
 * // Simple usage with a specific route
 * const wrapper = getRouterWrapper({ location: '/projects/123' });
 * render(<MyComponent />, { wrapper });
 * ```
 *
 * @example
 * ```tsx
 * // Complex usage with multiple wrappers
 * const routerWrapper = getRouterWrapper({
 *   location: '/projects/123/assistants/chat'
 * });
 * const themeWrapper = getThemeWrapper();
 *
 * const wrapper = buildWrapper([themeWrapper, routerWrapper]);
 * render(
 *   <ComponentUsingBothRouterAndTheme />,
 *   { wrapper }
 * );
 * ```
 */
export function getRouterWrapper(options?: { location?: string }) {
  const { location = '/' } = options ?? {};
  return function RouterWrapper({ children }: WrapperComponentProps) {
    return (
      <MemoryRouter initialEntries={[location]}>
        <Routes>
          <Route path=":projectId" element={children} />
          <Route path="*" element={children} />
        </Routes>
      </MemoryRouter>
    );
  };
}
