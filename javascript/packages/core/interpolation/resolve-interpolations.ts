import { isValidElement } from 'react';
import { mapValues } from 'lodash';

import { getObjectSymbols } from '#core/utils/object-utils';
import { Interpolation } from './base';
import { StringInterpolation } from './string-interpolation';

import type { InterpolationContext } from './types';

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
}): unknown {
  const { variable, params } = args;

  if (variable === null || variable === undefined || isValidElement(variable)) {
    return variable;
  }

  if (variable instanceof Interpolation) {
    const result = variable.interpolate(params) as unknown;
    // If interpolation didn't resolve, return as-is for future attempts
    return result === variable ? variable : resolveInterpolations({ variable: result, params });
  }

  if (StringInterpolation.isInterpolation(variable)) {
    return new StringInterpolation(variable as string).interpolate(params);
  }

  if (Array.isArray(variable)) {
    return variable.map((v) => resolveInterpolations({ variable: v, params }));
  }

  if (typeof variable === 'object' && variable !== null) {
    const symbols = getObjectSymbols(variable);
    const mappedValues = mapValues(variable as Record<string, unknown>, (v) =>
      resolveInterpolations({ variable: v, params })
    );

    // Preserve object symbols for framework compatibility
    return { ...mappedValues, ...symbols };
  }

  return variable;
}
