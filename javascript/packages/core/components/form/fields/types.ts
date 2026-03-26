import type { FieldValidator } from '#core/components/form/validation/types';

export interface BaseFieldProps<T = unknown, InputValue = T> {
  /** Unique ID of the field,
   *
   *  - Identifies the field input in the form
   *  - Corresponds to the data key in the form object
   *  - Supports dot notation for nested fields (e.g., "user.name", "spec.pipeline.name")
   *
   * Learn more:{@link https://final-form.org/docs/final-form/field-names}
   */
  name: string;

  /**
   * Label displayed above the field
   */
  label?: string;

  /**
   * The value of the field upon creation. Previously configured values take
   * precedence over default value.
   */
  defaultValue?: T;

  /**
   * The value of the field upon page load. Takes precedence over default and
   * configured values. Can be overwritten by user modification.
   */
  initialValue?: T;

  required?: boolean;

  /**
   * When true, the field maintains default styling but is immutable.
   *
   * The difference between disabled and readonly is that read-only controls can
   * still function, are still focusable, _and_ are still provided within form
   * submission request.
   */
  readOnly?: boolean;

  /**
   * When true, the field maintains adopts disabled styling and is immutable.
   * **Disabled fields will be filtered from the associated form submission request**
   */
  disabled?: boolean;

  /**
   * Specifies a short hint that describes the expected value of the field
   * **Placeholder text disappears after input is provided.**
   */
  placeholder?: string;

  /**
   * Information displayed when hovering the label
   * **Markdown supported.**
   */
  description?: string;

  /**
   * Information displayed directly below the field.
   * **Markdown supported.**
   */
  caption?: string;

  /**
   * Arbitrary content rendered at the far right of the label row.
   * Can be a React component, text, or an action button.
   */
  labelEndEnhancer?: React.ReactNode;

  /**
   * Transforms the input value before storing it in form state.
   * Called with the input value.
   */
  parse?: (value: InputValue) => T;

  /**
   * Transforms the field value for display in the input.
   * Called with the field value from form state.
   */
  format?: (value: T) => InputValue;

  /**
   * Validation function called on each value change after the field is touched.
   * Returns an error message string when invalid, or `undefined` when valid.
   * Use `combineValidators` to compose multiple validators.
   */
  validate?: FieldValidator;
}
