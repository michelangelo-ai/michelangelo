import { FunctionInterpolation } from './function-interpolation';
import { StringInterpolation } from './string-interpolation';

import type { StudioParamsView } from '#core/hooks/routing/use-studio-params/types';
import type { InterpolationContext } from './types';

/**
 * Creates interpolation patterns that resolve to dynamic values at runtime.
 *
 * @example
 * ```typescript
 * // String interpolation
 * const greeting = interpolate('Hello ${user.name}');
 * // When resolved with {user: {name: "John"}}: "Hello John"
 *
 * // Function interpolation
 * const dynamic = interpolate(({ page }) => `Editing ${page.title}`);
 * // When resolved with {page: {title: "Dashboard"}}: "Editing Dashboard"
 * ```
 */
export function interpolate<T, U extends StudioParamsView = 'base'>(
  input: string | ((params: InterpolationContext<U>) => T)
): T {
  return (
    typeof input === 'function' ? new FunctionInterpolation(input) : new StringInterpolation(input)
  ) as T;
}
