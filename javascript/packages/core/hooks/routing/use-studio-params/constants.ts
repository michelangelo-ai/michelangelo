import type { StudioParamsView } from '#core/types/common/view-types';
import type { ParamsTransformer } from './types';

/**
 * A mapping of view types to their corresponding parameter transformers.
 * Each transformer is responsible for converting base parameters into the
 * specific parameter type required by that view.
 *
 * The transformers handle three types of parameters:
 * 1. Base parameters (entity, phase, projectId)
 * 2. Route parameters from the URL path
 * 3. Query parameters from the URL search string
 *
 * @remarks
 * While the underlying view type should assume existence of the parameters, the
 * transformers provide fallback values to ensure that the parameters are correctly
 * typed and normalized.
 *
 * For example, a Schema Driven UI detail view should _always_ have an entityId. But
 * the URL parsing does not guarantee entityId's existence. Consequently, the
 * detail view transformer provides a fallback entityId value of an empty string.
 *
 * @example
 * ```typescript
 * // Transform parameters for a form view
 * const formParams = VIEW_TYPE_TO_PARAMS.form(
 *   baseParams,
 *   { entityId: 'model-123' },
 *   { entitySubType: 'pipeline' }
 * );
 *
 * // Transform parameters for a detail view
 * const detailParams = VIEW_TYPE_TO_PARAMS.detail(
 *   baseParams,
 *   { entityId: 'model-123', entityTab: 'overview' },
 *   { revisionId: 'rev-456' }
 * );
 * ```
 *
 * @see {@link ParamsTransformer} for the transformer function type
 * @see {@link StudioParamsViewType} for all available view types
 */
export const VIEW_TYPE_TO_PARAMS: Record<StudioParamsView, ParamsTransformer<StudioParamsView>> = {
  form: (routeParams, queryParams) => ({
    phase: routeParams.phase!,
    projectId: routeParams.projectId!,
    entity: routeParams.entity!,
    entityId: routeParams.entityId!,
    entitySubType: queryParams.entitySubType,
    revisionId: queryParams.revisionId,
  }),
  detail: (routeParams, queryParams) => ({
    phase: routeParams.phase!,
    projectId: routeParams.projectId!,
    entity: routeParams.entity!,
    entityId: routeParams.entityId!,
    entityTab: routeParams.entityTab!,
    revisionId: queryParams.revisionId,
    pipelineName: queryParams.pipelineName,
  }),
  list: (routeParams) => ({
    phase: routeParams.phase!,
    entity: routeParams.entity!,
    projectId: routeParams.projectId!,
  }),
  'form-detail': (routeParams, queryParams) => ({
    phase: routeParams.phase!,
    projectId: routeParams.projectId!,
    entity: routeParams.entity!,
    entityId: routeParams.entityId!,
    entityTab: routeParams.entityTab,
    entitySubType: queryParams.entitySubType,
    revisionId: queryParams.revisionId,
    pipelineName: queryParams.pipelineName,
  }),
  base: (routeParams, queryParams) => ({
    ...routeParams,
    ...queryParams,
  }),
  unregistered: (routeParams, queryParams) => ({
    ...routeParams,
    ...queryParams,
  }),
};
