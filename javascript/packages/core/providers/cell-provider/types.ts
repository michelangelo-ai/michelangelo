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
   * Custom cell renderers that extend the built-in CELL_RENDERERS.
   * These will be checked first before falling back to default behavior.
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
