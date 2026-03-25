import { get, has } from 'lodash';

import { Interpolation } from './base';
import { INTERPOLATION_PATTERN, SYNTAX_CHARS } from './constants';

import type { StudioParamsView } from '#core/types/common/view-types';
import type { InterpolationContext } from './types';

/**
 * Handles string-based interpolation by replacing ${variable.path} patterns
 * with values from the provided context.
 *
 * @example
 * ```typescript
 * const interpolation = new StringInterpolation('Hello ${page.title}');
 * const result = interpolation.interpolate({ page: { title: 'Dashboard' } });
 * // result: "Hello Dashboard"
 * ```
 */
export class StringInterpolation<U extends StudioParamsView = 'base'> extends Interpolation<
  string,
  string,
  U
> {
  /**
   * Check if a value contains interpolation patterns.
   */
  static isInterpolation(value: unknown): value is string {
    return typeof value === 'string' && INTERPOLATION_PATTERN.test(value);
  }

  /**
   * Execute string interpolation by replacing ${variable.path} patterns
   * with values from the context.
   */
  execute(params: Partial<InterpolationContext<U>>): string {
    return this.interpolator.replace(new RegExp(INTERPOLATION_PATTERN, 'g'), (match) => {
      const path = match.replace(SYNTAX_CHARS, '');
      if (!has(params, path))
        throw new Error('Insufficient data to resolve the string interpolation');
      return String(get(params, path));
    });
  }
}
