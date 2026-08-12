import { Form } from '#core/components/form/form';
import { ResolvedFormContent } from '#core/components/form/resolved-form-content';
import { applySubmitTransforms } from '#core/components/form/utils/apply-submit-transforms';
import { filterHiddenConditionFields } from '#core/components/form/utils/filter-hidden-condition-fields';

import type { FieldConfig, FormConfig } from '#core/components/form/types/config-types';
import type { DeepPartial } from '#core/types/utility-types';

type ConfigDrivenFormProps<T extends Record<string, unknown> = Record<string, unknown>> = {
  config: FormConfig<T>;
  onSubmit: (values: T) => void | object | Promise<object>;
  initialValues?: DeepPartial<T>;
};

/**
 * Renders a form from a declarative configuration.
 *
 * @example
 * ```tsx
 * const config: FormConfig<{ name: string }> = {
 *   fields: { name: { type: 'string', label: 'Name' } },
 *   layout: ['name'],
 * };
 *
 * <ConfigDrivenForm config={config} onSubmit={handleSubmit} />
 * ```
 */
export function ConfigDrivenForm<T extends Record<string, unknown>>({
  config,
  onSubmit,
  initialValues,
}: ConfigDrivenFormProps<T>) {
  return (
    <Form<T>
      onSubmit={(values, ...rest) => {
        const transformed = applySubmitTransforms([filterHiddenConditionFields], values, config);
        // cast: onSubmit's declared type only accepts `values`; forwarding the extra
        // final-form args (form, callback) preserves the pre-existing pass-through behavior
        return (onSubmit as (...args: unknown[]) => void | object | Promise<object>)(
          transformed,
          ...rest
        );
      }}
      initialValues={initialValues}
    >
      <ResolvedFormContent
        layout={config.layout}
        // cast: erases keyof T — layouts are structural and don't depend on the data shape
        fields={config.fields as Record<string, FieldConfig | undefined>}
      />
    </Form>
  );
}
