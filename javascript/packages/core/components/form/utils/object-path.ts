import { cloneDeep, get, isEmpty, set, toPath, unset } from 'lodash';

/**
 * Reads the value at a dot/bracket path (e.g. `spec.items[0].name`), or `undefined` if any
 * segment along the path is missing.
 *
 * Callers outside this file should never reach for a form library's own path resolver directly —
 * going through here keeps the form utilities swappable if the underlying form library ever
 * changes.
 */
export function getByPath(values: Record<string, unknown>, path: string): unknown {
  return get(values, path);
}

/**
 * Returns a copy of `values` with the value at `path` removed, pruning any parent object or
 * array left empty as a result.
 */
export function deleteByPath<T extends Record<string, unknown>>(values: T, path: string): T {
  const result = cloneDeep(values);
  unsetPath(result, path);
  return result;
}

/**
 * Deletes `path` from `obj` in place, then walks back up the path deleting any ancestor left
 * empty by the removal. If pruning empties out a slot in an array ancestor, compacts the array
 * rather than leaving a hole.
 */
function unsetPath(obj: object, path: string): void {
  const pathArray = toPath(path);
  if (pathArray.length === 0) return;

  do {
    unset(obj, pathArray);
    pathArray.pop();
  } while (pathArray.length > 0 && isEmpty(get(obj, pathArray)));

  const ancestor: unknown = get(obj, pathArray);
  if (Array.isArray(ancestor)) {
    set(
      obj,
      pathArray,
      ancestor.filter((item) => item !== undefined)
    );
  }
}
