import { isArray, values } from 'lodash';

import { isInterpolation } from './is-interpolation';

/**
 * Recursively checks if an object contains any interpolation patterns.
 *
 * @example
 * ```typescript
 * // Objects with interpolations
 * hasInterpolationProperty({
 *   title: interpolate('${page.name}'),
 *   count: 42
 * }); // true
 *
 * // Arrays with interpolations
 * hasInterpolationProperty([
 *   'static text',
 *   interpolate('${user.email}')
 * ]); // true
 *
 * // Nested structures
 * hasInterpolationProperty({
 *   metadata: {
 *     nested: {
 *       value: interpolate('${data.value}')
 *     }
 *   }
 * }); // true
 *
 * // No interpolations
 * hasInterpolationProperty({ name: 'John', age: 30 }); // false
 * ```
 */
export function hasInterpolationProperty(value: unknown): boolean {
  if (isInterpolation(value)) return true;

  if (isArray(value)) return value.some(hasInterpolationProperty);

  if (typeof value === 'object' && value !== null) {
    return values(value).some(hasInterpolationProperty);
  }

  return false;
}
