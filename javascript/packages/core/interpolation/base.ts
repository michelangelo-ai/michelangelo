import type { StudioParamsView } from '#core/hooks/routing/use-studio-params/types';
import type { InterpolationContext } from './types';

/**
 * Base class for transforming interpolation patterns into resolved values.
 *
 * @template InterpolatorType - The type of the interpolator (string | function)
 * @template InterpolationResult - The type returned by executing the interpolation
 * @template U - The studio params view type
 */
export abstract class Interpolation<
  InterpolatorType,
  InterpolationResult,
  U extends StudioParamsView = 'base',
> {
  cachedPage: any;
  cachedInitialValues: any;
  interpolator: InterpolatorType;

  constructor(interpolator: InterpolatorType) {
    this.interpolator = interpolator;
    this.cachedPage = undefined;
    this.cachedInitialValues = undefined;
  }

  interpolate(params: InterpolationContext<U>) {
    // Cache page and initialValues for subsequent resolutions
    if (params.page) this.cachedPage = params.page;
    if (params.initialValues) this.cachedInitialValues = params.initialValues;

    try {
      return this.execute({
        ...params,
        page: this.cachedPage,
        data: params.row ?? this.cachedPage,
        initialValues: this.cachedInitialValues,
      } as InterpolationContext<U>);
    } catch {
      // Return self for future resolution attempts
      return this;
    }
  }
  /**
   * Transforms the interpolator into its resolved value using the provided context.
   */
  abstract execute(params: Partial<InterpolationContext<U>>): InterpolationResult;
}
