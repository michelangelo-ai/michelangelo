/**
 * A function that validates a value and returns an error message if the value is invalid.
 *
 * @param value - The value to validate.
 * @returns An error message if the value is invalid, or `undefined` if the value is valid.
 *
 * @example
 * ```ts
 * const validate = (value: unknown) => {
 *   if (value === undefined) return 'Value is required';
 *   return undefined;
 * };
 * ```
 */
export type FieldValidator = (value: unknown) => string | undefined;
