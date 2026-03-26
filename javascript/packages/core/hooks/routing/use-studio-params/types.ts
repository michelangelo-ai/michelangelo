import { Phase } from '#core/types/common/studio-types';
import { StudioParamsView } from '#core/types/common/view-types';

/**
 * Parameters that can be extracted from the route path. These parameters
 * should be available in the Routes registered to the application's router.
 */
export type RouteParams = {
  /** The entity identifier (e.g., 'model', 'experiment') */
  entity: string;
  /** The specific ID of the entity being viewed */
  entityId: string;
  /** The current tab being viewed for the entity */
  entityTab: string;
  /** The current phase of the project (e.g., 'train', 'deploy') */
  phase: Phase;
  /** The ID of the project being viewed */
  projectId: string;
  /**
   * The ID of the revision being viewed. revisionId is only available in RouteParams
   * for project forms. For other forms, the revision ID is extracted from the query params.
   */
  revisionId: string;
};

/**
 * Parameters that can be extracted from the URL query string.
 */
export type QueryParams = {
  /** The subtype of the entity being viewed (e.g., specific pipeline type) */
  entitySubType: string;
  /** The ID of a specific revision being viewed */
  revisionId: string;

  /**
   * Non-standardized query params. This allows for useStudioParams callers
   * to access query params that are unique to their use case and should not
   * be relied on globally.
   */
  [key: string]: string;
};

/**
 * Base parameters required for all studio views. A base studio view
 * is a view defined within the Schema Driven UI. These views are either
 * a 'list' view, a 'detail' view, or a 'form' view.
 */
export type StudioParamsBase = Pick<RouteParams, 'entity' | 'phase' | 'projectId'>;

/**
 * Parameters specific to form views.
 *
 * entitySubType is only available for form views that are configured to support multiple
 * entity subtypes. @see {@link FormViewT} for more information.
 *
 * revisionId is only available for form views that modify a revisioned entity. @see
 * {@link REVISION_TYPE} for more information.
 */
export type StudioParamsForm = StudioParamsBase &
  Partial<Pick<QueryParams, 'entitySubType' | 'revisionId'>> &
  Pick<RouteParams, 'entityId'>;

/**
 * Parameters specific to detail views.
 *
 * revisionId is only available for detail views that modify a revisioned entity. @see
 * {@link REVISION_TYPE} for more information.
 *
 * pipelineName is a special query param that is used to identify the pipeline in
 * alert detail views. @see {@link PIPELINE_LEVEL_ALERT_FORM_SCHEMA} for more information.
 */
export type StudioParamsDetail = StudioParamsBase &
  Partial<Pick<QueryParams, 'revisionId' | 'pipelineName'>> &
  Pick<RouteParams, 'entityId' | 'entityTab'>;

/**
 * Parameters for combined form-detail views.
 *
 * There are lots of components that are used in form and detail views. This type
 * ensures that common fields between form and detail views are handled consistently.
 * For example, the form and detail views both use the entityId field.
 */
export type StudioParamsFormDetail = Pick<
  StudioParamsForm,
  Extract<keyof StudioParamsForm, keyof StudioParamsDetail>
> &
  Pick<StudioParamsDetail, Extract<keyof StudioParamsForm, keyof StudioParamsDetail>> &
  Partial<Omit<StudioParamsForm, Extract<keyof StudioParamsForm, keyof StudioParamsDetail>>> &
  Partial<Omit<StudioParamsDetail, Extract<keyof StudioParamsForm, keyof StudioParamsDetail>>>;

/**
 * Maps a view type to its corresponding parameter type.
 *
 * @template T - The view type to get parameters for
 * @example
 * ```typescript
 * // Gets form view parameters
 * type FormParams = ViewTypeToParamType<'form'>;
 * // Gets detail view parameters
 * type DetailParams = ViewTypeToParamType<'detail'>;
 * ```
 */
export type ViewTypeToParamType<T extends StudioParamsView> = T extends 'form'
  ? StudioParamsForm
  : T extends 'detail'
    ? StudioParamsDetail
    : T extends 'list'
      ? StudioParamsBase
      : T extends 'base'
        ? StudioParamsBase & Partial<RouteParams> & Partial<QueryParams>
        : T extends 'form-detail'
          ? StudioParamsFormDetail
          : T extends 'unregistered'
            ? Record<string, string | undefined>
            : never;

/**
 * A function type that transforms base parameters into view-specific parameters.
 * Used to create consistent parameter transformations for different view types.
 *
 * @template V - The view type to transform parameters for
 * @param baseParams - The base parameters to transform
 * @param routeParams - The route parameters available
 * @param queryParams - The query parameters available
 * @returns Parameters specific to the view type
 *
 * @example
 * ```typescript
 * const formTransformer: ParamsTransformer<'form'> = (base, params, query) => ({
 *   ...base,
 *   entityId: params.entityId ?? '',
 *   entitySubType: query.entitySubType
 * });
 * ```
 */
export type ParamsTransformer<V extends StudioParamsView> = (
  routeParams: Partial<RouteParams>,
  queryParams: Partial<QueryParams>
) => ViewTypeToParamType<V>;
