import { useCallback } from 'react';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { resolveInterpolations } from './resolve-interpolations';

import type { InterpolationContext, UserDataSources } from './types';

/**
 * React hook that returns a function to resolve interpolations in an unknown
 * data structure. It combines data from multiple sources, including React Contexts,
 * URL parameters, and page data.
 *
 * @example
 * ```typescript
 * // Basic usage, react context includes user.name = "John"
 * const resolve = useInterpolationResolver();
 * const greeting = resolve(interpolate('Hello ${user.name}'));
 * // greeting: "Hello John"
 *
 * // With additional data sources
 * const schema = interpolate('${page.title}: ${row.id}');
 * const result = resolve(schema, {
 *   page: { title: 'Dashboard' },
 *   row: { id: 123 }
 * });
 * // result: "Dashboard: 123"
 * ```
 */
export function useInterpolationResolver() {
  const studio = useStudioParams('base');

  return useCallback(
    <T = unknown>(variable: T, input?: Partial<UserDataSources>): T => {
      const minimumInterpolationData: InterpolationContext = {
        studio,
        data: undefined,
        page: undefined,
        initialValues: undefined,
        response: undefined,
        row: undefined,
        ...input,
      };

      return resolveInterpolations({
        variable,
        params: { ...minimumInterpolationData, ...input },
      }) as T;
    },
    [studio]
  );
}
