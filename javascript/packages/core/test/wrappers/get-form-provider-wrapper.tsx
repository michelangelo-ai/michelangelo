import { Form } from '#core/components/form/form';

import type { FormProps } from '#core/components/form/types';
import type { WrapperComponentProps } from './types';

export function getFormProviderWrapper(
  props?: Partial<Pick<FormProps, 'onSubmit' | 'initialValues'>>
) {
  const { onSubmit = vi.fn(), initialValues = {} } = props ?? {};
  return function FormProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <Form onSubmit={onSubmit} initialValues={initialValues}>
        {children}
      </Form>
    );
  };
}
