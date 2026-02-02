import { useCallback } from 'react';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useInterpolationContext } from '#core/providers/interpolation-provider/use-interpolation-context';
import { useRepeatedLayoutContext } from '#core/providers/repeated-layout-provider/use-repeated-layout-context';
import { resolveInterpolations } from './resolve-interpolations';

import type { ExclusionCheck, InterpolationContext, UserDataSources } from './types';

/**
 * Returns a function to resolve interpolated values by merging data from multiple sources
 * including URL params, React contexts, and user-provided data.
 *
 * The resolver traverses data structures recursively, replacing interpolated values
 * (created with `interpolate()`) with their resolved values. Non-interpolated values
 * pass through unchanged.
 *
 * Data sources are merged in this priority order (later overrides earlier):
 * 1. Studio route params (projectId, phase, etc.)
 * 2. Repeated layout context (for nested/repeated UI elements)
 * 3. Injected interpolation context (from InterpolationProvider)
 * 4. User-provided data sources (passed as second argument)
 *
 * @returns Function that resolves interpolations:
 *   - `variable`: The value or data structure to resolve (can be any type)
 *   - `input`: Optional additional data sources (page, row, data, etc.)
 *   - `excludeProperty`: Optional function to exclude certain properties from resolution
 *
 * @example
 * ```typescript
 * // Basic string interpolation
 * const resolve = useInterpolationResolver();
 * const greeting = resolve(interpolate('Hello ${user.name}'));
 * // If context has user.name = "John": "Hello John"
 *
 * // Object with interpolated values
 * const config = {
 *   title: interpolate('${page.title}'),
 *   id: interpolate('${row.id}'),
 *   static: 'unchanged'
 * };
 * const resolved = resolve(config, {
 *   page: { title: 'Dashboard' },
 *   row: { id: 123 }
 * });
 * // Returns: { title: 'Dashboard', id: 123, static: 'unchanged' }
 *
 * // Function interpolation
 * const dynamic = interpolate(({ studio }) => studio.projectId);
 * const projectId = resolve(dynamic);
 * // Returns current projectId from URL params
 *
 * // Excluding properties from resolution
 * const data = { config: interpolate('${value}'), schema: rawSchema };
 * const resolved = resolve(
 *   data,
 *   { value: 'resolved' },
 *   (key) => key === 'schema' // Exclude 'schema' from resolution
 * );
 * // Returns: { config: 'resolved', schema: rawSchema }
 * ```
 */
export function useInterpolationResolver() {
  const injectedContext = useInterpolationContext();
  const repeatedLayoutContext = useRepeatedLayoutContext();
  const studio = useStudioParams('base');

  return useCallback(
    <T = unknown>(
      variable: T,
      input?: Partial<UserDataSources>,
      excludeProperty?: ExclusionCheck
    ): T => {
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
        excludeProperty,
      }) as T;
    },
    [injectedContext, repeatedLayoutContext, studio]
  );
}
