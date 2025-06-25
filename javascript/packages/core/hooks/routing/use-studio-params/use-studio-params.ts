import React from 'react';
import { useParams } from 'react-router-dom-v5-compat';

import { useURLQueryString } from '#core/hooks/routing/use-url-query-string';
import { Phase } from '#core/types/common/studio-types';
import { VIEW_TYPE_TO_PARAMS } from './constants';
import { normalizeEntityParam } from './normalize-entity-param';

import type { QueryParams, RouteParams, StudioParamsView, ViewTypeToParamType } from './types';

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
  const queryParams = useURLQueryString<QueryParams>();

  return React.useMemo(() => {
    if (!params.phase) {
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
    return transformer(normalizeEntityParam(params), queryParams) as ViewTypeToParamType<T>;
  }, [params, queryParams, viewType]);
}
