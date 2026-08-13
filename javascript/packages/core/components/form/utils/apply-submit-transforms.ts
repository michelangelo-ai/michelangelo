import type { FormConfig } from '#core/components/form/types/config-types';
import type { SubmitTransform } from './types';

/** Runs `values` through each `transforms` step in order, threading the result forward. */
export function applySubmitTransforms<T extends Record<string, unknown>>(
  transforms: SubmitTransform<T>[],
  values: T,
  config: FormConfig<T>
): T {
  return transforms.reduce((acc, transform) => transform(acc, config), values);
}
