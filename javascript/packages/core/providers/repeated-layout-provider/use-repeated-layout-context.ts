import { useContext } from 'react';

import { RepeatedLayoutContext } from './repeated-layout-context';

/**
 * Accesses the current state of nested/repeated UI layouts for interpolation.
 *
 * This hook is used internally by useInterpolationResolver to provide context
 * about which iteration of a repeated layout is being rendered. This enables
 * interpolations to access the current item in a repeated section.
 *
 * Returns undefined if used outside of a RepeatedLayoutProvider.
 *
 * @returns Repeated layout state containing the current item and index, or undefined
 *
 * @example
 * ```typescript
 * // Inside a repeated layout (e.g., rendering a list)
 * function RepeatedItem() {
 *   const layoutContext = useRepeatedLayoutContext();
 *
 *   // layoutContext might be: { currentItem: {...}, index: 2 }
 *   // This allows interpolations to reference the current item
 * }
 * ```
 */
export const useRepeatedLayoutContext = () => useContext(RepeatedLayoutContext);
