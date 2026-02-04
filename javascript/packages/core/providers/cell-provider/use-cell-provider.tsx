import { useContext } from 'react';

import { CellContext } from './cell-context';

/**
 * Accesses custom cell renderers registered via CellProvider.
 *
 * This hook must be used within a CellProvider component. It provides access
 * to custom cell renderers that override or extend the default cell rendering behavior.
 *
 * @returns Cell context containing custom renderer mappings, or undefined if no CellProvider exists
 *
 * @example
 * ```typescript
 * function useCustomCellRenderer() {
 *   const cellContext = useCellProvider();
 *
 *   // Check if custom renderer exists for a type
 *   if (cellContext?.renderers['custom-type']) {
 *     return cellContext.renderers['custom-type'];
 *   }
 *
 *   // Fall back to default renderer
 *   return DefaultCellRenderer;
 * }
 * ```
 */
export const useCellProvider = () => {
  return useContext(CellContext);
};
