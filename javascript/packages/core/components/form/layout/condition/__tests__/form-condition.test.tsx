import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { Form } from '#core/components/form/form';
import { useForm } from '#core/components/form/hooks/use-form';
import { FormCondition } from '#core/components/form/layout/condition/form-condition';
import { RepeatedLayoutProvider } from '#core/providers/repeated-layout-provider/repeated-layout-provider';
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

  describe('within a repeated layout', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'items.name',
      is: 'admin',
      items: [],
    };

    it('resolves the condition against the indexed field for the current repeated item', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ items: [{ name: 'guest' }, { name: 'admin' }] }}>
          <RepeatedLayoutProvider rootFieldPath="items" index={1}>
            <FormCondition layout={layout}>
              <div>Conditional Content</div>
            </FormCondition>
          </RepeatedLayoutProvider>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('does not match against a different index in the same repeated list', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ items: [{ name: 'guest' }, { name: 'admin' }] }}>
          <RepeatedLayoutProvider rootFieldPath="items" index={0}>
            <FormCondition layout={layout}>
              <div>Conditional Content</div>
            </FormCondition>
          </RepeatedLayoutProvider>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('reacts to changes at the indexed field, not the unindexed entity path', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ items: [{ name: 'guest' }] }}>
          <RepeatedLayoutProvider rootFieldPath="items" index={0}>
            <FormCondition layout={layout}>
              <div>Conditional Content</div>
            </FormCondition>
            <SetValueButton name="items[0].name" value="admin" label="Change value" />
          </RepeatedLayoutProvider>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Change value' }));

      await waitFor(() => expect(screen.queryByText('Conditional Content')).toBeInTheDocument());
    });
  });

  describe('isNot', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'mode',
      isNot: 'hidden',
      items: [],
    };

    it('hides children when value equals isNot', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'hidden' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('renders children when value differs from isNot and is non-empty', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'shown' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('hides children when value is empty ("not yet determined")', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: '' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('hides children again once value changes back to isNot', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'shown' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="mode" value="hidden" label="Change value" />
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

  describe('isEmpty: true', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'name',
      isEmpty: true,
      items: [],
    };

    it('renders children when field is empty', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: '' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('hides children when field is non-empty', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: 'Alice' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('hides children once field becomes non-empty, and shows again once cleared', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: '' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="name" value="Alice" label="Set to Alice" />
          <SetValueButton name="name" value="" label="Clear" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Set to Alice' }));
      await waitFor(() =>
        expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument()
      );

      await user.click(screen.getByRole('button', { name: 'Clear' }));
      await waitFor(() => expect(screen.queryByText('Conditional Content')).toBeInTheDocument());
    });
  });

  describe('isEmpty: false', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'name',
      isEmpty: false,
      items: [],
    };

    it('renders children when field is non-empty', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: 'Alice' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('hides children when field is empty', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: '' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('hides children again once field is cleared', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: 'Alice' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="name" value="" label="Clear" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Clear' }));

      await waitFor(() =>
        expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument()
      );
    });
  });

  describe('containsAny', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'role',
      containsAny: ['admin', 'superadmin'],
      items: [],
    };

    it('renders when a scalar value is in the list', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ role: 'admin' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('hides when a scalar value is not in the list', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ role: 'guest' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('renders when an array value overlaps with the list', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ role: ['guest', 'admin'] }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).toBeInTheDocument();
    });

    it('hides when an array value does not overlap with the list', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ role: ['guest', 'viewer'] }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByText('Conditional Content')).not.toBeInTheDocument();
    });

    it('hides again once the value changes to a non-matching one', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ role: 'admin' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="role" value="guest" label="Change value" />
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
