import { getIn, setIn } from 'final-form';

/**
 * Reads the value at a dot/bracket path (e.g. `spec.items[0].name`), or `undefined` if any
 * segment along the path is missing.
 *
 * Backed by final-form's path resolver so nested and indexed paths behave exactly like they do
 * elsewhere in the form (field names, `when` conditions). Callers outside this file should never
 * import from `final-form` directly — going through here keeps the form utilities swappable if
 * the underlying form library ever changes.
 */
export function getByPath(values: Record<string, unknown>, path: string): unknown {
  return getIn(values, path);
}

/**
 * Returns a copy of `values` with the value at `path` removed, pruning any parent object that's
 * left empty as a result.
 */
export function deleteByPath<T extends Record<string, unknown>>(values: T, path: string): T {
  // cast: final-form's setIn is typed loosely as `(state: object, key: string, value: any) =>
  // object`; it always returns a value with the same shape as `values`
  return (setIn(values, path, undefined) ?? {}) as T;
}
