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
   * Cell renderers registered at the application level. Checked before built-in
   * renderers, so a registered renderer for a known CellType will override the
   * default. Use this for app-wide customization. For per-column overrides, use
   * the column-level `Cell` prop instead.
   *
   * @example
   * ```tsx
   * const renderers = {
   *   'CUSTOM_BADGE': MyBadgeRenderer,
   *   [CellType.BOOLEAN]: MyBooleanRenderer,
   * };
   * ```
   */
  renderers: Record<string, CellRenderer<unknown>>;
};
