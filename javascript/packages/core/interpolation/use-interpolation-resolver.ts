import { useCallback } from 'react';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useInterpolationContext } from '#core/providers/interpolation-provider/use-interpolation-context';
import { useRepeatedLayoutContext } from '#core/providers/repeated-layout-provider/use-repeated-layout-context';
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
  const injectedContext = useInterpolationContext();
  const repeatedLayoutContext = useRepeatedLayoutContext();
  const studio = useStudioParams('base');

  return useCallback(
    <T = unknown>(variable: T, input?: Partial<UserDataSources>): T => {
      const minimumInterpolationData: InterpolationContext = {
        studio,
        repeatedLayoutContext,
        data: undefined,
        page: undefined,
        initialValues: undefined,
        response: undefined,
        row: undefined,
        ...input,
        ...injectedContext,
      };

      return resolveInterpolations({
        variable,
        params: { ...minimumInterpolationData, ...input },
      }) as T;
    },
    [injectedContext, repeatedLayoutContext, studio]
  );
}
