import { CellContext } from './cell-context';

import type { CellContextType } from './types';

/**
 * @description
 * Provider component that allows consumers to register custom cell renderers.
 * These custom renderers will be checked before falling back to built-in renderers.
 *
 * @example
 * ```tsx
 * const customRenderers = {
 *   'CUSTOM_BADGE': MyBadgeRenderer,
 *   'SPECIAL_TYPE': MySpecialRenderer
 * };
 *
 * <CellProvider renderers={customRenderers}>
 *   <MyTable />
 * </CellProvider>
 * ```
 */
export const CellProvider = ({
  children,
  renderers = {},
}: { children: React.ReactNode } & Partial<CellContextType>) => {
  const contextValue: CellContextType = {
    renderers,
  };

  return <CellContext.Provider value={contextValue}>{children}</CellContext.Provider>;
};
