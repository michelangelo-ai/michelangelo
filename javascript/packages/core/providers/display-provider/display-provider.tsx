import { DisplayContext } from './display-context';

import type { DisplayContextType } from './types';

/**
 * Declares the display region for everything rendered inside it. Nest a
 * narrower `DisplayProvider` to override a subregion.
 *
 * @example
 * ```tsx
 * <DisplayProvider type="nav">
 *   <BreadcrumbBar ... />
 * </DisplayProvider>
 * ```
 */
export function DisplayProvider({
  type,
  children,
}: {
  type: DisplayContextType;
  children: React.ReactNode;
}) {
  return <DisplayContext.Provider value={type}>{children}</DisplayContext.Provider>;
}
