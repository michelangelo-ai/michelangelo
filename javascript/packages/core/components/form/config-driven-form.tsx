import { Form } from '#core/components/form/form';
import { LayoutItemList } from '#core/components/form/layout/layout-item-list';

import type { FormConfig } from '#core/components/form/types/config-types';
import type { FormData } from '#core/components/form/types/form-types';

type ConfigDrivenFormProps = {
  config: FormConfig;
  onSubmit: (values: FormData) => void | object | Promise<object>;
  initialValues?: Record<string, unknown>;
};

export function ConfigDrivenForm({ config, onSubmit, initialValues }: ConfigDrivenFormProps) {
  return (
    <Form onSubmit={onSubmit} initialValues={initialValues}>
      <LayoutItemList items={config.layout} fields={config.fields} />
    </Form>
  );
}
