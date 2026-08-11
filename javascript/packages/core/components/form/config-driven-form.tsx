import { Form } from '#core/components/form/form';
import { LayoutItemList } from '#core/components/form/layout/layout-item-list';

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
    <Form<T> onSubmit={onSubmit} initialValues={initialValues}>
      <LayoutItemList
        items={config.layout}
        // cast: erases keyof T — layouts are structural and don't depend on the data shape
        fields={config.fields as Record<string, FieldConfig | undefined>}
      />
    </Form>
  );
}
