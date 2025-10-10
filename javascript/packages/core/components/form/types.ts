import type { DeepPartial } from '#core/types/utility-types';

export type FormData = Record<string, unknown>;

export interface FormProps<FieldValues extends FormData = FormData> {
  onSubmit: (values: FieldValues) => void | object | Promise<object>;
  initialValues?: DeepPartial<FieldValues>;

  /** Form ID for external submit button integration */
  id?: string;
  children: React.ReactNode;

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
}

export interface FormState<FieldValues extends FormData = FormData> {
  submitting: boolean;
  submitError?: string;
  values?: FieldValues;
}

export interface FieldState {
  error?: string;
  touched: boolean;
}

export interface FieldInput<T = unknown> {
  value: T;
  name: string;
  onChange: (value: T) => void;
  onBlur: () => void;
}
