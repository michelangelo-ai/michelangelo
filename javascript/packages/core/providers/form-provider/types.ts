import type { FieldRenderer } from '#core/components/form/types/config-types';

/**
 * @description
 * The form context provided to the application to extend built-in field
 * renderers with custom ones. Custom renderers are checked first before
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
};
