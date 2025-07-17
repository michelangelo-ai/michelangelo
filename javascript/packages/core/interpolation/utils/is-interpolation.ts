import { Interpolation } from '../base';
import { StringInterpolation } from '../string-interpolation';

/**
 * Checks if a value contains interpolation patterns.
 *
 * @example
 * ```typescript
 * // String interpolation patterns
 * isInterpolation('Hello ${user.name}'); // true
 * isInterpolation('Hello world'); // false
 *
 * // Interpolation instances
 * isInterpolation(interpolate('${page.title}')); // true
 * isInterpolation(interpolate(({ data }) => data.count)); // true
 *
 * // Other values
 * isInterpolation(42); // false
 * isInterpolation(null); // false
 * ```
 */
export function isInterpolation(value: unknown): boolean {
  return value instanceof Interpolation || StringInterpolation.isInterpolation(value);
}
