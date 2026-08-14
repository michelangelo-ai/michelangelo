import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { Form } from '#core/components/form/form';
import { useForm } from '#core/components/form/hooks/use-form';
import { FormCondition } from '#core/components/form/layout/condition/form-condition';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

import type { ConditionLayoutConfig } from '#core/components/form/layout/condition/types';

function SetValueButton({ name, value, label }: { name: string; value: unknown; label: string }) {
  const { change } = useForm();
  return <button onClick={() => change(name, value)}>{label}</button>;
}

describe('FormCondition', () => {
  describe('is', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'mode',
      is: 'advanced',
      items: [],
    };

    it('renders children when value matches', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'advanced' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('hides children when value does not match', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'basic' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('hides children again when value changes away from the match', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'advanced' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="mode" value="basic" label="Change value" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Change value' }));

      await waitFor(() =>
        expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument()
      );
    });
  });
});
