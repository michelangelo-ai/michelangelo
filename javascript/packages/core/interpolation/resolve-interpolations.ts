import { isValidElement } from 'react';
import { mapValues } from 'lodash';

import { getObjectSymbols } from '#core/utils/object-utils';
import { Interpolation } from './base';
import { StringInterpolation } from './string-interpolation';

import type { ExclusionCheck, InterpolationContext } from './types';

/**
 * Processes any data structure, resolving interpolation patterns into actual values.
 *
 * @example
 * ```typescript
 * // String interpolation
 * const result = resolveInterpolations({
 *   variable: interpolate('Hello ${user.name}'),
 *   params: { user: { name: 'John' } }
 * });
 * // Returns: "Hello John"
 *
 * // Nested object resolution
 * const schema = {
 *   title: interpolate('${page.title}'),
 *   items: [interpolate('${data.count}'), 'static']
 * };
 * const resolved = resolveInterpolations({
 *   variable: schema,
 *   params: { page: { title: 'Dashboard' }, data: { count: 5 } }
 * });
 * // Returns: { title: "Dashboard", items: [5, "static"] }
 * ```
 */
export function resolveInterpolations(args: {
  variable: unknown;
  params: InterpolationContext;
  excludeProperty?: ExclusionCheck;
}): unknown {
  const { variable, params, excludeProperty } = args;

  if (variable === null || variable === undefined || isValidElement(variable)) {
    return variable;
  }

  if (variable instanceof Interpolation) {
    const result = variable.interpolate(params) as unknown;
    // If interpolation didn't resolve, return as-is for future attempts
    return result === variable
      ? variable
      : resolveInterpolations({ variable: result, params, excludeProperty });
  }

  if (StringInterpolation.isInterpolation(variable)) {
    return new StringInterpolation(variable).interpolate(params);
  }

  if (Array.isArray(variable)) {
    return variable.map((v) =>
      resolveInterpolations({
        variable: v,
        params,
        excludeProperty,
      })
    );
  }

  if (typeof variable === 'object' && variable !== null) {
    const symbols = getObjectSymbols(variable);
    const mappedValues = mapValues(variable, (value, key) => {
      try {
        if (excludeProperty?.(key, value)) {
          return value;
        }
      } catch {
        console.warn('Failed to exclude property from interpolation', key, value);
      }

      return resolveInterpolations({
        variable: value,
        params,
        excludeProperty,
      });
    });

    // Preserve object symbols for framework compatibility
    return { ...mappedValues, ...symbols };
  }

  return variable;
}
