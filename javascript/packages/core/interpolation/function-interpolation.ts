import { Interpolation } from './base';

import type { StudioParamsView } from '#core/hooks/routing/use-studio-params/types';
import type { InterpolationContext } from './types';

/**
 * Handles function-based interpolation by calling the function with the
 * interpolation context provided as argument.
 *
 * @example
 * ```typescript
 * const interpolation = new FunctionInterpolation(({ page }) => `Editing ${page.title}`);
 * const result = interpolation.interpolate({ page: { title: 'Dashboard' } });
 * // result: "Editing Dashboard"
 * ```
 */
export class FunctionInterpolation<
  InterpolationResult,
  U extends StudioParamsView = 'base',
> extends Interpolation<
  (params: InterpolationContext<U>) => InterpolationResult,
  InterpolationResult,
  U
> {
  execute(params: Partial<InterpolationContext<U>>): InterpolationResult {
    return this.interpolator(params as InterpolationContext<U>);
  }
}
