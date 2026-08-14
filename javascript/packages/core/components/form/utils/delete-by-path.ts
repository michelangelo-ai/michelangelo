import { cloneDeep, get, isEmpty, set, toPath, unset } from 'lodash';

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
