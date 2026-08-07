import type { ComponentType } from 'react';
import type { StringFieldConfig } from '#core/components/form/fields/string/types';
import type { SharedFieldConfig } from '#core/components/form/fields/types';
import type { FormGridLayoutConfig } from '#core/components/form/layout/form-grid/types';
import type { FormGroupLayoutConfig } from '#core/components/form/layout/form-group/types';
import type { FormRowLayoutConfig } from '#core/components/form/layout/form-row/types';

/** Enumerates the built-in field types available in the config-driven form system. */
export enum FieldType {
  /**
   * @description Renders a single-line **Input** or, with `multi: true`, a **Tag input** for multiple values
   */
  STRING = 'string',
}

/**
 * Module-augmentation extension point for consumer field configs.
 *
 * @example
 * ```ts
 * declare module '@michelangelo-ai/core' {
 *   interface FieldConfigExtensions {
 *     'select-hive': { type: 'select-hive'; cluster: string };
 *   }
 * }
 * ```
 */
// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface FieldConfigExtensions {}

export type BuiltinFieldConfig = StringFieldConfig;

/** Union of all field config types — built-in and consumer-extended. */
export type FieldConfig<T = unknown> =
  | BuiltinFieldConfig
  | (SharedFieldConfig<T> & { type: string });

/** Props passed to a field renderer by the config-driven form system. */
export type FieldRendererProps = {
  name: string;
  config: FieldConfig;
};

/** A React component that renders a form field from its config. */
export type FieldRenderer = ComponentType<FieldRendererProps>;

/** Union of layout types that the form engine renders. */
export type LayoutConfig = FormGroupLayoutConfig | FormRowLayoutConfig | FormGridLayoutConfig;

/** A layout entry — either a layout config or a field path string. */
export type LayoutItem = LayoutConfig | string;

/**
 * Declarative form configuration that defines fields and their layout.
 * Generic `T` constrains field keys and value types (defaultValue, parse, format)
 * to match the form's data shape.
 */
export type FormConfig<T extends Record<string, unknown> = Record<string, unknown>> = {
  fields: { [K in keyof T]?: FieldConfig<T[K]> };
  layout: LayoutItem[];
};
