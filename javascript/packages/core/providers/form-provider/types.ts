import type { FieldRenderer, LayoutRenderer } from '#core/components/form/types/config-types';

/**
 * @description
 * The form context provided to the application to extend built-in field and
 * layout renderers with custom ones. Custom renderers are checked first before
 * falling back to built-in behavior.
 */
export type FormContextType = {
  /**
   * @description
   * Field renderers registered at the application level. Checked before built-in
   * renderers, so a registered renderer for a known FieldType will override the
   * default.
   *
   * @example
   * ```tsx
   * const renderers = {
   *   'hive-select': HiveSelectField,
   *   [FieldType.STRING]: MyCustomStringField,
   * };
   * ```
   */
  renderers: Record<string, FieldRenderer>;

  /**
   * @description
   * Layout renderers registered at the application level. Checked before built-in
   * layout types (group, row, grid), so consumers can add new layout types or
   * override existing ones.
   *
   * @example
   * ```tsx
   * const layouts = {
   *   tabs: TabsLayoutRenderer,
   *   accordion: AccordionLayoutRenderer,
   * };
   * ```
   */
  layouts: Record<string, LayoutRenderer>;
};
