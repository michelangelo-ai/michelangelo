import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { ConfigDrivenForm } from '#core/components/form/config-driven-form';
import { FormProvider } from '#core/providers/form-provider/form-provider';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

import type { FieldRendererProps, FormConfig } from '#core/components/form/types/config-types';

describe('ConfigDrivenForm', () => {
  describe('field rendering', () => {
    const config: FormConfig = {
      fields: {
        name: { type: 'string', label: 'Name' },
        email: { type: 'string', label: 'Email' },
      },
      layout: ['name', 'email'],
    };

    beforeEach(() => {
      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
    });

    it('renders a string field', () => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument();
    });

    it('renders all configured fields', () => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument();
      expect(screen.getByLabelText('Email')).toBeInTheDocument();
    });
  });

  describe('form submission', () => {
    it('submits values from config-driven fields', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name' },
          email: { type: 'string', label: 'Email' },
        },
        layout: ['name', 'email'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={onSubmit} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Name'), 'Alice');
      await user.type(screen.getByLabelText('Email'), 'alice@example.com');
      fireEvent.submit(screen.getByLabelText('Email'));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { name: 'Alice', email: 'alice@example.com' },
          expect.anything(),
          expect.anything()
        )
      );
    });
  });

  describe('layout composition', () => {
    it('renders fields inside a group layout', () => {
      const config: FormConfig = {
        fields: { name: { type: 'string', label: 'Name' } },
        layout: [{ type: 'group', title: 'User Info', items: ['name'] }],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByText('User Info')).toBeInTheDocument();
      expect(screen.getByLabelText('Name')).toBeInTheDocument();
    });

    it('renders fields inside a row layout', () => {
      const config: FormConfig = {
        fields: {
          first: { type: 'string', label: 'First' },
          last: { type: 'string', label: 'Last' },
        },
        layout: [{ type: 'row', name: 'Full Name', items: ['first', 'last'] }],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByText('Full Name')).toBeInTheDocument();
      expect(screen.getByLabelText('First')).toBeInTheDocument();
      expect(screen.getByLabelText('Last')).toBeInTheDocument();
    });

    it('renders nested layouts', () => {
      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name' },
          email: { type: 'string', label: 'Email' },
        },
        layout: [
          {
            type: 'group',
            title: 'Contact',
            items: [{ type: 'row', items: ['name', 'email'] }],
          },
        ],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByText('Contact')).toBeInTheDocument();
      expect(screen.getByLabelText('Name')).toBeInTheDocument();
      expect(screen.getByLabelText('Email')).toBeInTheDocument();
    });
  });

  describe('field resolution', () => {
    it('renders nothing for unregistered field types', () => {
      const config: FormConfig = {
        fields: { custom: { type: 'unknown-type', label: 'Custom' } },
        layout: ['custom'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByLabelText('Custom')).not.toBeInTheDocument();
    });

    it('only renders fields referenced in layout', () => {
      const config: FormConfig = {
        fields: {
          visible: { type: 'string', label: 'Visible' },
          hidden: { type: 'string', label: 'Hidden' },
        },
        layout: ['visible'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByLabelText('Visible')).toBeInTheDocument();
      expect(screen.queryByLabelText('Hidden')).not.toBeInTheDocument();
    });

    it('renders nothing for layout paths with no matching field config', () => {
      const config: FormConfig = {
        fields: {},
        layout: ['nonexistent'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    });
  });

  describe('FormProvider integration', () => {
    it('uses custom renderer from FormProvider over built-in', () => {
      const CustomStringField = ({ name, config }: FieldRendererProps) => (
        <div data-testid="custom-string">
          Custom: {config.type} for {name}
        </div>
      );

      const config: FormConfig = {
        fields: { name: { type: 'string', label: 'Name' } },
        layout: ['name'],
      };

      render(
        <FormProvider renderers={{ string: CustomStringField }}>
          <ConfigDrivenForm config={config} onSubmit={vi.fn()} />
        </FormProvider>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      // eslint-disable-next-line testing-library/no-test-id-queries -- mock component, no semantic role
      expect(screen.getByTestId('custom-string')).toHaveTextContent('Custom: string for name');
      expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    });

    it('renders consumer-registered custom field types', () => {
      const HiveSelectField = ({ name }: FieldRendererProps) => (
        <label>
          Hive Cluster
          <select name={name}>
            <option>cluster-1</option>
          </select>
        </label>
      );

      const config: FormConfig = {
        fields: { cluster: { type: 'hive-select', label: 'Hive Cluster' } },
        layout: ['cluster'],
      };

      render(
        <FormProvider renderers={{ 'hive-select': HiveSelectField }}>
          <ConfigDrivenForm config={config} onSubmit={vi.fn()} />
        </FormProvider>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByLabelText('Hive Cluster')).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'cluster-1' })).toBeInTheDocument();
    });
  });

  describe('field properties', () => {
    it('applies placeholder from config', () => {
      const config: FormConfig = {
        fields: { name: { type: 'string', label: 'Name', placeholder: 'Enter name' } },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByPlaceholderText('Enter name')).toBeInTheDocument();
    });

    it('marks required fields', () => {
      const config: FormConfig = {
        fields: { name: { type: 'string', label: 'Name', required: true } },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByText('*')).toBeInTheDocument();
    });

    it('renders disabled fields', () => {
      const config: FormConfig = {
        fields: { name: { type: 'string', label: 'Name', disabled: true } },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByLabelText('Name')).toBeDisabled();
    });

    it('pre-populates fields with initial values', () => {
      const config: FormConfig = {
        fields: { name: { type: 'string', label: 'Name' } },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm
          config={config}
          onSubmit={vi.fn()}
          initialValues={{ name: 'Pre-filled' }}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByLabelText('Name')).toHaveValue('Pre-filled');
    });
  });
});
