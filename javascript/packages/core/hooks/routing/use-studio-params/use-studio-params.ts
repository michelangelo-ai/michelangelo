import React from 'react';
import { useParams } from 'react-router-dom-v5-compat';

import { useUrlQueryString } from '#core/hooks/routing/use-url-query-string';
import { Phase } from '#core/types/common/studio-types';
import { VIEW_TYPE_TO_PARAMS } from './constants';
import { normalizeEntityParam } from './normalize-entity-param';

import type { StudioParamsView } from '#core/types/common/view-types';
import type { QueryParams, RouteParams, ViewTypeToParamType } from './types';

/**
 * Hook to get and transform studio parameters based on the view type.
 *
 * @template T - The view type to get parameters for. Defaults to 'unregistered'
 * @param viewType - Must match the type parameter T
 * @returns Parameters specific to the view type
 *
 * @example
 * ```typescript
 * // With explicit type
 * const formParams = useStudioParams('form');
 *
 * // Using default type
 * const unregisteredParams = useStudioParams('unregistered');
 * ```
 */
export function useStudioParams<T extends StudioParamsView = 'unregistered'>(
  viewType: T
): ViewTypeToParamType<T> {
  const params: Partial<RouteParams> = useParams();
  const queryParams = useUrlQueryString<QueryParams>();

  return React.useMemo(() => {
    if (!params.phase) {
      // cast: mapped type can't be satisfied without a cast from this concrete shape; whether the
      // no-phase path is reachable for every T (e.g. 'detail', which requires entityTab) is
      // unverified; see #1459
      return {
        ...queryParams,
        revisionId: params.revisionId,
        projectId: params.projectId!,
        phase: Phase.Project,
        entity: 'projects',
        entityId: params.projectId!,
      } as ViewTypeToParamType<T>;
    }

    const transformer = VIEW_TYPE_TO_PARAMS[viewType];
    // cast: transformer is selected by viewType; return value always matches ViewTypeToParamType<T>
    // at runtime
    return transformer(normalizeEntityParam(params), queryParams) as ViewTypeToParamType<T>;
  }, [params, queryParams, viewType]);
}
