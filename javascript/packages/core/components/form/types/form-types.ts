import type { FORM_ERROR } from 'final-form';
import type { ComponentType } from 'react';
import type { DeepPartial } from '#core/types/utility-types';

export type FormData = Record<string, unknown>;

export interface FormProps<FieldValues extends FormData = FormData> {
  onSubmit: (values: FieldValues) => void | object | Promise<object>;
  initialValues?: DeepPartial<FieldValues>;

  /** Form ID for external submit button integration */
  id?: string;
  children: React.ReactNode;

  /**
   * When true, focuses the first field with a validation error on failed submit.
   *
   * @default true
   */
  focusOnError?: boolean;

  /**
   * Optional render prop for wrapping the form element.
   * When provided, the form element is passed to this function, allowing
   * components outside the form element to access form state via useFormState.
   */
  render?: (formElement: React.ReactNode) => React.ReactNode;

  /**
   * Renders a sticky footer fixed to the bottom of the viewport.
   *
   * @note `right` is usually reserved for form actions (e.g., submit button).
   * @note `left` is usually reserved for secondary info, status text.
   */
  footer?: { left?: React.ReactNode; right?: React.ReactNode } | React.ReactNode;
}

export interface FormInstance {
  fieldRegistry: FieldRegistry;
}

/**
 * `FORM_ERROR` is final-form's own convention for form-level submission errors:
 * any `onSubmit` can return `{ [FORM_ERROR]: ... }` directly (typically a string
 * message), or an `Error`. Preserving `Error` (rather than collapsing to its message)
 * keeps properties like an error code available to consumers that want to render
 * more specific messaging than the raw message string.
 */
export type SubmitErrors = { [FORM_ERROR]?: string | Error } & Record<string, unknown>;

export interface FormState<FieldValues extends FormData = FormData> {
  submitting: boolean;
  submitError?: string | Error;
  values?: FieldValues;
  submitFailed?: boolean;
  hasValidationErrors?: boolean;
  errors?: Record<string, unknown>;
  submitErrors?: SubmitErrors;
  touched?: Record<string, boolean>;
  modifiedSinceLastSubmit?: boolean;
}

export interface FieldState {
  error?: string;
  touched: boolean;
}

export interface FieldInput<T = unknown, InputValue = T> {
  value: InputValue;
  name: string;
  onChange: (value: InputValue) => void;
  onBlur: () => void;
  onFocus: () => void;
}

export type FieldRegistry = Map<string, FieldRegistryEntry>;

export type FieldRegistryEntry = { label: string };

export interface FormApi {
  fieldRegistry: FieldRegistry;
  change: (name: string, value: unknown) => void;
  submit: () => Promise<object | undefined> | undefined;
}
export interface ArrayFieldOptions {
  /**
   * Pre-populates with empty entries _on mount_ when the array has fewer items than this value,
   * and prevents removal when the array has fewer items than this value.
   */
  minItems?: number;
  readOnly?: boolean;
}

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

export type SharedFieldConfig = {
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

export type FieldRendererProps = {
  name: string;
  config: FieldConfig;
};

export type FieldRenderer = ComponentType<FieldRendererProps>;

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

export type LayoutRendererProps = {
  config: LayoutConfig;
  renderItems: (items: LayoutItem[]) => React.ReactNode;
};

export type LayoutRenderer = ComponentType<LayoutRendererProps>;

export type FormConfig = {
  entities: Record<string, FieldConfig>;
  layout: LayoutItem[];
};
