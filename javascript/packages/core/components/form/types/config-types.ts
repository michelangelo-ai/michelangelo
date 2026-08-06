import type { ComponentType } from 'react';
import type { StringFieldConfig } from '#core/components/form/fields/string/types';
import type { SharedFieldConfig } from '#core/components/form/fields/types';

/** Enumerates the built-in field types available in the config-driven form system. */
export enum FieldType {
  /** Single-line or multi-line text input */
  STRING = 'string',
  /** Numeric input */
  NUMBER = 'number',
  /** Toggle or checkbox for true/false values */
  BOOLEAN = 'boolean',
  /** Dropdown selection from a list of options */
  SELECT = 'select',
  /** Multiple-choice checkbox group */
  CHECKBOX = 'checkbox',
  /** Radio button group for single selection */
  RADIO = 'radio',
  /** Date picker */
  DATE = 'date',
  /** Multi-line text area */
  TEXTAREA = 'textarea',
  /** URL input with validation */
  URL = 'url',
  /** Key-value pair editor */
  MAP = 'map',
  /** Markdown editor with preview */
  MARKDOWN = 'markdown',
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

type GroupLayoutConfig = {
  type: 'group';
  label?: string;
  description?: string;
  tooltip?: string;
  collapsible?: boolean;
  items: LayoutItem[];
};

type RowLayoutConfig = {
  type: 'row';
  name?: string;
  span?: number[];
  items: LayoutItem[];
};

type GridLayoutConfig = {
  type: 'grid';
  items: LayoutItem[];
};

/** Union of built-in layout types that the form engine renders natively. */
export type BuiltinLayoutConfig = GroupLayoutConfig | RowLayoutConfig | GridLayoutConfig;

/** A layout node — either a built-in type or a consumer-registered custom type. */
export type LayoutConfig = BuiltinLayoutConfig | { type: string; items: LayoutItem[] };

/** A layout tree node — either a layout config object or a bare field path string. */
export type LayoutItem = LayoutConfig | string;

/** Props passed to a layout renderer by the config-driven form system. */
export type LayoutRendererProps = {
  config: LayoutConfig;
  renderItems: (items: LayoutItem[]) => React.ReactNode;
};

/** A React component that renders a layout node from its config. */
export type LayoutRenderer = ComponentType<LayoutRendererProps>;

/**
 * Declarative form configuration that defines fields and their layout.
 * Generic `T` constrains field keys and value types (defaultValue, parse, format)
 * to match the form's data shape.
 */
export type FormConfig<T extends Record<string, unknown> = Record<string, unknown>> = {
  fields: { [K in keyof T]?: FieldConfig<T[K]> };
  layout: LayoutItem[];
};
