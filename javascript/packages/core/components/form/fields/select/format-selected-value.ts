/**
 * Parse {value} from {T | T[]} into an array of {T}
 *
 * @remarks
 * Baseui expects the `value` to be an array, but select fields can persist single values.
 */
export function formatSelectedValue<T>(value: Array<T> | T): Array<T> {
  if (value) {
    return Array.isArray(value) ? value : [value];
  }

  return [];
}
