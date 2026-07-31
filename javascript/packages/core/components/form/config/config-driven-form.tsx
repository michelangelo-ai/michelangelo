import { Form } from '#core/components/form/form';
import { LayoutItemList } from './layout/layout-item-list';

import type { FormData } from '#core/components/form/types';
import type { FormConfig } from './types';

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
