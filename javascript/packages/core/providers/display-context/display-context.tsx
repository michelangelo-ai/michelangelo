import { DisplayContextValue } from './display-context-context';

import type { DisplayContextType } from './types';

/**
 * Declares the display region ("nav" or "content") for everything rendered
 * inside it, so components never decide their own entity-name casing — see
 * `useEntityName`. Nest a narrower `DisplayContext` to override a subregion,
 * or pass an explicit casing to `useEntityName` for a one-off exception.
 *
 * @example
 * ```tsx
 * <DisplayContext type="nav">
 *   <BreadcrumbBar ... />
 * </DisplayContext>
 * ```
 */
export function DisplayContext({
  type,
  children,
}: {
  type: DisplayContextType;
  children: React.ReactNode;
}) {
  return <DisplayContextValue.Provider value={type}>{children}</DisplayContextValue.Provider>;
}
