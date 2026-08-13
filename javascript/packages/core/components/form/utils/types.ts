import type { FormConfig } from '#core/components/form/types/config-types';

export type SubmitTransform<T extends Record<string, unknown>> = (
  values: T,
  config: FormConfig<T>
) => T;
