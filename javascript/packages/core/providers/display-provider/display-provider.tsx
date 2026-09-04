import { DisplayContext } from './display-context';

import type { DisplayContextType } from './types';

/**
 * Declares the display region ("nav" or "content") for everything rendered
 * inside it, so components never decide their own entity-name casing — see
 * `useEntityName`. Nest a narrower `DisplayProvider` to override a subregion,
 * or pass an explicit casing to `useEntityName` for a one-off exception.
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
