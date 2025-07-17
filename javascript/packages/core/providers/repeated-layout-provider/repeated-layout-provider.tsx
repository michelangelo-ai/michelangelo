import { RepeatedLayoutContext } from './repeated-layout-context';

import type { ReactNode } from 'react';
import type { RepeatedLayoutState } from './types';

/**
 * Provides repeated layout context for interpolations within repeated components.
 *
 * @example
 * ```tsx
 * // Within a repeated list where each item needs its index
 * <RepeatedLayoutProvider index={2} rootFieldPath="items">
 *   <ItemComponent />
 * </RepeatedLayoutProvider>
 *
 * // Inside ItemComponent, interpolations can access:
 * const title = interpolate('Item ${repeatedLayoutContext.index}');
 * // Resolves to: "Item 2"
 * ```
 */
export function RepeatedLayoutProvider({
  children,
  ...state
}: RepeatedLayoutState & { children: ReactNode }) {
  return <RepeatedLayoutContext.Provider value={state}>{children}</RepeatedLayoutContext.Provider>;
}
