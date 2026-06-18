import { isNil, mapValues } from 'lodash';

import { isInterpolation } from './is-interpolation';

/**
 * Recursively removes unresolved interpolation objects from data structures.
 *
 * @example
 * ```typescript
 * const schema = {
 *   title: 'Dashboard',
 *   user: interpolate('${user.name}'), // unresolved
 *   config: {
 *     theme: 'dark',
 *     greeting: interpolate('${welcome.message}') // unresolved
 *   }
 * };
 *
 * const cleaned = clearUnresolvedInterpolations(schema);
 * // Returns: {
 * //   title: 'Dashboard',
 * //   user: undefined,
 * //   config: {
 * //     theme: 'dark',
 * //     greeting: undefined
 * //   }
 * // }
 * ```
 *
 * @remarks
 * Does not recursively process array values - only object properties.
 * Unresolved interpolations become `undefined` rather than being removed entirely.
 */
export function clearUnresolvedInterpolations<T extends object>(input: T): T {
  // cast: mapValues returns a partial object type; result has the same keys as T with values cleaned of interpolations
  return mapValues<T>(input, (value: unknown) => {
    if (isNil(value) || Array.isArray(value)) return value;

    if (isInterpolation(value)) return undefined;

    if (typeof value === 'object') return clearUnresolvedInterpolations(value as T); // cast: value passed the object check; T extends object so the recursive call is safe

    return value;
  }) as T;
}
