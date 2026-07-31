import type { ComponentType } from 'react';
import type { BaseFieldProps } from '#core/components/form/fields/types';

// ---------------------------------------------------------------------------
// Field types
// ---------------------------------------------------------------------------

export enum FieldType {
  STRING = 'string',
  NUMBER = 'number',
  BOOLEAN = 'boolean',
  SELECT = 'select',
  CHECKBOX = 'checkbox',
  RADIO = 'radio',
  DATE = 'date',
  TEXTAREA = 'textarea',
  URL = 'url',
  MAP = 'map',
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

type SharedFieldConfig = {
  label?: string;
  required?: boolean;
  disabled?: boolean;
  readOnly?: boolean;
  placeholder?: string;
  description?: string;
  caption?: string;
  defaultValue?: unknown;
  initialValue?: unknown;
};

type StringFieldConfig = SharedFieldConfig & {
  type: FieldType.STRING | 'string';
  multi?: boolean;
};

export type BuiltinFieldConfig = StringFieldConfig;

export type FieldConfig = BuiltinFieldConfig | (SharedFieldConfig & { type: string });

// ---------------------------------------------------------------------------
// Field renderer
// ---------------------------------------------------------------------------

export type FieldRendererProps = BaseFieldProps & {
  config: Record<string, unknown>;
};

export type FieldRenderer = ComponentType<FieldRendererProps>;

// ---------------------------------------------------------------------------
// Layout types
// ---------------------------------------------------------------------------

/**
 * Module-augmentation extension point for consumer layout types.
 *
 * @example
 * ```ts
 * declare module '@michelangelo-ai/core' {
 *   interface LayoutConfigExtensions {
 *     tabs: { type: 'tabs'; items: LayoutItem[] };
 *   }
 * }
 * ```
 */
// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface LayoutConfigExtensions {}

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

export type BuiltinLayoutConfig = GroupLayoutConfig | RowLayoutConfig | GridLayoutConfig;

export type LayoutConfig = BuiltinLayoutConfig | { type: string; items: LayoutItem[] };

/** A layout item is either a layout config object or a bare field path string. */
export type LayoutItem = LayoutConfig | string;

// ---------------------------------------------------------------------------
// Layout renderer
// ---------------------------------------------------------------------------

export type LayoutRendererProps = {
  config: LayoutConfig;
  renderItems: (items: LayoutItem[]) => React.ReactNode;
};

export type LayoutRenderer = ComponentType<LayoutRendererProps>;

// ---------------------------------------------------------------------------
// Form config
// ---------------------------------------------------------------------------

export type FormConfig = {
  entities: Record<string, FieldConfig>;
  layout: LayoutItem[];
};
