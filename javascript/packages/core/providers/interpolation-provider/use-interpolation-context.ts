import { useContext } from 'react';

import { InterpolationContext } from './interpolation-context';

/**
 * Accesses custom data sources injected via InterpolationProvider for use in
 * interpolation resolution.
 *
 * This hook is primarily used internally by useInterpolationResolver to merge
 * provider-level data sources with component-level data. Returns an empty object
 * if used outside of an InterpolationProvider.
 *
 * @returns Interpolation context data (page, data, row, etc.) or empty object
 *
 * @example
 * ```typescript
 * function MyComponent() {
 *   const context = useInterpolationContext();
 *
 *   // context might contain: { page: {...}, data: {...}, row: {...} }
 *   // These values are available for interpolation resolution
 * }
 * ```
 */
export const useInterpolationContext = () => useContext(InterpolationContext) ?? {};
