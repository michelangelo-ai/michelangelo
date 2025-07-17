import { StringInterpolation } from './string-interpolation';

import type { StudioParamsView } from '#core/hooks/routing/use-studio-params/types';

/**
 * Factory function to create interpolation instances based on input type.
 * Currently supports string interpolation.
 *
 * @example
 * ```typescript
 * // String interpolation
 * const greeting = interpolate('Hello ${user.name}');
 * ```
 */
export function interpolate<U extends StudioParamsView = 'base'>(
  template: string
): StringInterpolation<U> {
  return new StringInterpolation(template);
}
