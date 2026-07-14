import type { CellRenderer } from '#core/components/cell/types';

/**
 * @description
 * The cell context provided to the application to extend built-in cell renderers
 * with custom ones. Custom renderers are checked first before falling back to
 * built-in behavior.
 */
export type CellContextType = {
  /**
   * @description
   * Renderers for custom (application-defined) cell types that extend the
   * built-in set. Registered renderers are used as a fallback after built-in
   * renderers are checked, so this map cannot override a built-in CellType.
   * To render a specific column differently, use the column-level `Cell` prop.
   *
   * @example
   * ```tsx
   * const renderers = {
   *   'CUSTOM_BADGE': MyBadgeRenderer,
   *   'SPECIAL_TYPE': MySpecialRenderer
   * };
   * ```
   */
  renderers: Record<string, CellRenderer<unknown>>;
};
