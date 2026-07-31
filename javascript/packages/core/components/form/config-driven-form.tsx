import { Form } from '#core/components/form/form';
import { LayoutItemList } from '#core/components/form/layout/layout-item-list';

import type { FormConfig, FormData } from '#core/components/form/types';

type ConfigDrivenFormProps = {
  config: FormConfig;
  onSubmit: (values: FormData) => void | object | Promise<object>;
  initialValues?: Record<string, unknown>;
};

export function ConfigDrivenForm({ config, onSubmit, initialValues }: ConfigDrivenFormProps) {
  return (
    <Form onSubmit={onSubmit} initialValues={initialValues}>
      <LayoutItemList items={config.layout} entities={config.entities} />
    </Form>
  );
}
