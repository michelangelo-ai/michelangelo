import type {
  StudioParamsView,
  ViewTypeToParamType,
} from '#core/hooks/routing/use-studio-params/types';

/**
 * Base interface for data sources that users can provide to interpolation.
 * These represent the core data that drives interpolation logic.
 *
 * @example
 * ```typescript
 * const dataSources: UserDataSources = {
 *   page: { title: 'Dashboard', metadata: { name: 'John' } },
 *   row: { id: 123, status: 'active', priority: 8 },
 *   initialValues: { fallback: 'Default Value' },
 *   response: { success: true, timestamp: '2024-01-01' }
 * };
 * ```
 */
export interface UserDataSources {
  /**
   * Page is the data driving the highest-level view, e.g.
   * the data for the Detail view and Form view.
   */
  page: any;
  /**
   * InitialValues is the data before any changes were made to the form data.
   * This can be used to compare the current form state to the original data.
   */
  initialValues: any;
  /**
   * The response is what Unified API returns following a mutation request.
   * Given a successful action operation, response can be used to access Unified API response.
   */
  response: any;
  /**
   * Row is populated by list views and tables. Most table column interpolations
   * will reference row. Row also drives table actions.
   */
  row: any;
  /**
   * Endpoint that was invoked to generate the {@link response} interpolation property
   */
  endpoint?: string;
}

/**
 * The complete context object that interpolation functions receive as their argument.
 * Includes all user data sources plus computed values and framework context.
 *
 * @template U - The studio params view type
 *
 * @example
 * ```typescript
 * const interpolation = interpolate<string, 'form'>(
 *   (context: InterpolationContext<'form'>) => {
 *     const title = context.page.title;
 *     const priority = context.row.priority;
 *
 *     // Computed data (row ?? page)
 *     const primaryData = context.data;
 *
 *     // Framework context
 *     const phase = context.studio.phase;
 *
 *     return `${phase}: ${title}`;
 *   }
 * );
 * ```
 */
export interface InterpolationContext<U extends StudioParamsView = 'base'> extends UserDataSources {
  /**
   * Studio is the MA Studio-specific data picked from the URL, e.g. projectId,
   * phase, entity.
   */
  studio: ViewTypeToParamType<U>;
  /**
   * Data is resolved from page and row. It prefers row but will fall back to
   * page if row is not populated. This is used in actions schemas, since
   * actions leverage page data in the detail view and row data in the list view.
   */
  data: any;
}
