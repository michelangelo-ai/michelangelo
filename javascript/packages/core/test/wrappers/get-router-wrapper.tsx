import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom-v5-compat';

import type { WrapperComponentProps } from './types';

/**
 * Creates a React Router wrapper for testing components that use routing features.
 * This wrapper is essential for testing components that use react-router hooks
 * like useParams, useStudioParams, etc. Without this wrapper, tests will fail
 * with errors like "Cannot read properties of undefined (reading 'match')".
 *
 * @param options - Configuration options for the router
 * @param options.location - Initial URL path to render (defaults to '/')
 * @param options.initialEntries - Full history stack for the router (overrides location)
 * @param options.initialIndex - Index into initialEntries to start at (defaults to last entry)
 * @param options.showNavButtons - Render "Browser back" and "Browser forward" buttons for navigation testing
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
 * // Testing back/forward navigation with a pre-populated history stack
 * render(
 *   <MyComponent />,
 *   buildWrapper([
 *     getRouterWrapper({
 *       initialEntries: ['/projects', '/projects/123'],
 *       initialIndex: 1,
 *       showNavButtons: true,
 *     }),
 *   ])
 * );
 * await user.click(screen.getByRole('button', { name: 'Browser back' }));
 * ```
 */
export function getRouterWrapper(options?: {
  location?: string;
  initialEntries?: string[];
  initialIndex?: number;
  showNavButtons?: boolean;
}) {
  const { location = '/', initialEntries, initialIndex, showNavButtons = false } = options ?? {};
  const entries = initialEntries ?? [location];

  return function RouterWrapper({ children }: WrapperComponentProps) {
    return (
      <MemoryRouter initialEntries={entries} initialIndex={initialIndex}>
        <Routes>
          {[
            ':projectId/:phase/:entity/:entityId/:entityTab?',
            ':projectId/:phase/:entity?',
            ':projectId',
            '*',
          ].map((path) => (
            <Route
              key={path}
              path={path}
              element={
                <>
                  <ShowLocation showNavButtons={showNavButtons} />
                  {children}
                </>
              }
            />
          ))}
        </Routes>
      </MemoryRouter>
    );
  };
}

function ShowLocation({ showNavButtons }: { showNavButtons: boolean }) {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <div>
      <span>
        Current pathname: {location.pathname} {location.search}
      </span>
      <span>Current search: {location.search}</span>
      {showNavButtons && (
        <>
          <button onClick={() => navigate(-1)}>Browser back</button>
          <button onClick={() => navigate(1)}>Browser forward</button>
        </>
      )}
    </div>
  );
}
