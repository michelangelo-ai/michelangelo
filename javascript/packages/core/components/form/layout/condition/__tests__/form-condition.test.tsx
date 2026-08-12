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

function SetValueButton({ name, value }: { name: string; value: unknown }) {
  const { change } = useForm();
  return <button onClick={() => change(name, value)}>{`set:${JSON.stringify(value)}`}</button>;
}

function content() {
  return screen.queryByText('Conditional Content');
}

async function setValue(user: ReturnType<typeof userEvent.setup>, value: unknown) {
  await user.click(screen.getByRole('button', { name: `set:${JSON.stringify(value)}` }));
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

      expect(content()).toBeInTheDocument();
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

      expect(content()).not.toBeInTheDocument();
    });

    it('hides children again when value changes away from the match', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'advanced' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="mode" value="basic" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(content()).toBeInTheDocument();

      await setValue(user, 'basic');

      await waitFor(() => expect(content()).not.toBeInTheDocument());
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

      expect(content()).not.toBeInTheDocument();
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

      expect(content()).toBeInTheDocument();
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

      expect(content()).not.toBeInTheDocument();
    });

    it('hides children again once value changes back to isNot', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ mode: 'shown' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="mode" value="hidden" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(content()).toBeInTheDocument();

      await setValue(user, 'hidden');

      await waitFor(() => expect(content()).not.toBeInTheDocument());
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

      expect(content()).toBeInTheDocument();
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

      expect(content()).not.toBeInTheDocument();
    });

    it('hides children once field becomes non-empty, and shows again once cleared', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: '' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="name" value="Alice" />
          <SetValueButton name="name" value="" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(content()).toBeInTheDocument();

      await setValue(user, 'Alice');
      await waitFor(() => expect(content()).not.toBeInTheDocument());

      await setValue(user, '');
      await waitFor(() => expect(content()).toBeInTheDocument());
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

      expect(content()).toBeInTheDocument();
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

      expect(content()).not.toBeInTheDocument();
    });

    it('hides children again once field is cleared', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ name: 'Alice' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="name" value="" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(content()).toBeInTheDocument();

      await setValue(user, '');

      await waitFor(() => expect(content()).not.toBeInTheDocument());
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

      expect(content()).toBeInTheDocument();
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

      expect(content()).not.toBeInTheDocument();
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

      expect(content()).toBeInTheDocument();
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

      expect(content()).not.toBeInTheDocument();
    });

    it('hides again once the value changes to a non-matching one', async () => {
      const user = userEvent.setup();
      render(
        <Form onSubmit={vi.fn()} initialValues={{ role: 'admin' }}>
          <FormCondition layout={layout}>
            <div>Conditional Content</div>
          </FormCondition>
          <SetValueButton name="role" value="guest" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(content()).toBeInTheDocument();

      await setValue(user, 'guest');

      await waitFor(() => expect(content()).not.toBeInTheDocument());
    });
  });
});
