export interface BaseFieldProps {
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
}
