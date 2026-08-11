import { useMemo } from 'react';

import { useFormState } from '#core/components/form/hooks/use-form-state';
import { LayoutItemList } from '#core/components/form/layout/layout-item-list';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';

import type {
  FieldConfig,
  FormConfig,
  FormConfigSchema,
} from '#core/components/form/types/config-types';

/** Resolves interpolation expressions in the form config against current form values. */
export function ResolvedFormContent({ config }: { config: FormConfigSchema }) {
  const resolve = useInterpolationResolver();
  const { values } = useFormState({ values: true });

  const resolved = useMemo(
    // cast: resolver returns unknown; always FormConfig after interpolation
    () => resolve(config, { page: values }) as FormConfig,
    [resolve, config, values]
  );

  return (
    <LayoutItemList
      items={resolved.layout}
      // cast: erases keyof T — layouts are structural and don't depend on the data shape
      fields={resolved.fields as Record<string, FieldConfig | undefined>}
    />
  );
}
