import type { ComponentType } from 'react';
import type { BooleanFieldConfig } from '#core/components/form/fields/boolean/types';
import type { CheckboxFieldConfig } from '#core/components/form/fields/checkbox/types';
import type { DateFieldConfig } from '#core/components/form/fields/date/types';
import type { MapFieldConfig } from '#core/components/form/fields/map/types';
import type { MarkdownFieldConfig } from '#core/components/form/fields/markdown/types';
import type { NumberFieldConfig } from '#core/components/form/fields/number/types';
import type { RadioFieldConfig } from '#core/components/form/fields/radio/types';
import type { SelectFieldConfig } from '#core/components/form/fields/select/types';
import type { StringFieldConfig } from '#core/components/form/fields/string/types';
import type { TextareaFieldConfig } from '#core/components/form/fields/textarea/types';
import type { SharedFieldConfig } from '#core/components/form/fields/types';
import type { UrlFieldConfig } from '#core/components/form/fields/url/types';
import type { ConditionLayoutConfig } from '#core/components/form/layout/condition/types';
import type { FormGridLayoutConfig } from '#core/components/form/layout/form-grid/types';
import type { FormGroupLayoutConfig } from '#core/components/form/layout/form-group/types';
import type { FormRowLayoutConfig } from '#core/components/form/layout/form-row/types';

/**
 * Declarative form configuration that defines fields and their layout.
 * Generic `T` constrains field keys and value types (defaultValue, parse, format)
 * to match the form's data shape.
 */
export type FormConfig<T extends Record<string, unknown> = Record<string, unknown>> = {
  fields: { [K in keyof T]?: FieldConfig<T[K]> };
  layout: LayoutItem[];
};

/** Union of all field config types — built-in and consumer-extended. */
export type FieldConfig<T = unknown> =
  | BuiltinFieldConfig
  | (SharedFieldConfig<T> & { type: string });

/** A layout entry — either a layout config or a field path string. */
export type LayoutItem = LayoutConfig | string;

/** Union of layout types that the form engine renders. */
export type LayoutConfig =
  | FormGroupLayoutConfig
  | FormRowLayoutConfig
  | FormGridLayoutConfig
  | ConditionLayoutConfig;

/** Union of all field config types implemented by packages/core. */
export type BuiltinFieldConfig =
  | StringFieldConfig
  | NumberFieldConfig
  | BooleanFieldConfig
  | SelectFieldConfig
  | CheckboxFieldConfig
  | RadioFieldConfig
  | DateFieldConfig
  | TextareaFieldConfig
  | UrlFieldConfig
  | MapFieldConfig
  | MarkdownFieldConfig;

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

/**
 * Declarative validation config that maps to the built-in validators in `validation/validators.ts`.
 * Each key resolves to its corresponding validator factory via `resolveValidation`.
 */
export type BuiltinFieldValidation = {
  regex?: { pattern: string | RegExp; errorMessage?: string };
  min?: number;
  max?: number;
  minLength?: number;
  maxLength?: number;
  url?: { errorMessage?: string } | boolean;
};

/**
 * Full validation config combining declarative rules with an optional custom validator.
 * Declarative rules are resolved first; the custom `validate` function runs last.
 */
export type FieldValidation = BuiltinFieldValidation & {
  /** Custom validator that runs after all declarative rules. */
  validate?: (value: unknown) => string | undefined;
};

/** Enumerates the built-in field types available in the config-driven form system. */
export enum FieldType {
  /**
   * @description Renders a single-line **Input** or, with `multi: true`, a **Tag input** for multiple values
   */
  STRING = 'string',

  /**
   * @description Renders a numeric **Input** (`type="number"`)
   */
  NUMBER = 'number',

  /**
   * @description Renders a **Checkbox** or, with `toggle: true`, a **Toggle** switch
   */
  BOOLEAN = 'boolean',

  /**
   * @description Renders a **Select** dropdown with searchable options
   */
  SELECT = 'select',

  /**
   * @description Renders a **CheckboxGroup** for multiple selections
   */
  CHECKBOX = 'checkbox',

  /**
   * @description Renders a **RadioGroup** for single selection
   */
  RADIO = 'radio',

  /**
   * @description Renders a **DatePicker** with calendar popover
   */
  DATE = 'date',

  /**
   * @description Renders a multi-line **Textarea**
   */
  TEXTAREA = 'textarea',

  /**
   * @description Renders a read-only **Link** when the value is a navigable URL, otherwise a placeholder
   */
  URL = 'url',

  /**
   * @description Renders a **key-value editor** with add/remove rows
   */
  MAP = 'map',

  /**
   * @description Renders a **Textarea** in edit mode or formatted **Markdown** in read-only mode
   */
  MARKDOWN = 'markdown',
}

/** Props passed to a field renderer by the config-driven form system. */
export type FieldRendererProps = {
  name: string;
  config: FieldConfig;
};

/** A React component that renders a form field from its config. */
export type FieldRenderer = ComponentType<FieldRendererProps>;
