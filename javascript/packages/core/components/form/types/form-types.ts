import type { FORM_ERROR } from 'final-form';
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
   *
   * @example
   * ```tsx
   * // Form with external submit button in wrapper
   * <Form
   *   id="my-form"
   *   onSubmit={handleSubmit}
   *   render={(formElement) => (
   *     <div>
   *       {formElement}
   *       <footer>
   *         <button type="submit" form="my-form">Submit</button>
   *       </footer>
   *     </div>
   *   )}
   * >
   *   <StringField name="email" label="Email" />
   * </Form>
   *
   * // Standalone form (no render prop needed)
   * <Form onSubmit={handleSubmit}>
   *   <StringField name="email" label="Email" />
   *   <button type="submit">Submit</button>
   * </Form>
   * ```
   */
  render?: (formElement: React.ReactNode) => React.ReactNode;

  /**
   * Renders a sticky footer fixed to the bottom of the viewport.
   *
   * @note `right` is usually reserved for form actions (e.g., submit button).
   * @note `left` is usually reserved for secondary info, status text.
   *
   * @example
   * ```tsx
   * // Object with left and right content
   * <Form footer={{ right: <SubmitButton>Save</SubmitButton>, left: <span>Last saved 2m ago</span> }}>
   *
   * // ReactNode for full control
   * <Form footer={<MyCustomFooter />}>
   * ```
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
