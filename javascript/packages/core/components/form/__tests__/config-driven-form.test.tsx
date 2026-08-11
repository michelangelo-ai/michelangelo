import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { ConfigDrivenForm } from '#core/components/form/config-driven-form';
import { interpolate } from '#core/interpolation/interpolate';
import { FormProvider } from '#core/providers/form-provider/form-provider';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';

import type {
  FieldRendererProps,
  FormConfig,
  FormConfigSchema,
} from '#core/components/form/types/config-types';

describe('ConfigDrivenForm', () => {
  describe('field rendering', () => {
    const config: FormConfig = {
      fields: {
        name: { type: 'string', label: 'Name' },
        age: { type: 'number', label: 'Age' },
        active: { type: 'boolean', label: 'Active', checkboxLabel: 'Is Active' },
        role: { type: 'select', label: 'Role', options: [{ id: 'admin', label: 'Admin' }] },
        permissions: {
          type: 'checkbox',
          label: 'Permissions',
          options: [{ id: 'read', label: 'Read' }],
        },
        size: { type: 'radio', label: 'Size', options: [{ value: 'sm', label: 'Small' }] },
        startDate: { type: 'date', label: 'Start Date' },
        notes: { type: 'textarea', label: 'Notes' },
        website: { type: 'url', label: 'Website', placeholder: 'No URL' },
        tags: { type: 'map', label: 'Tags' },
        content: { type: 'markdown', label: 'Content' },
      },
      layout: [
        'name',
        'age',
        'active',
        'role',
        'permissions',
        'size',
        'startDate',
        'notes',
        'website',
        'tags',
        'content',
      ],
    };

    beforeEach(() => {
      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders a string field', () => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument();
    });

    it('renders a number field', () => {
      expect(screen.getByRole('spinbutton')).toBeInTheDocument();
    });

    it('renders a boolean field', () => {
      expect(screen.getByRole('checkbox', { name: 'Is Active' })).toBeInTheDocument();
    });

    it('renders a select field', () => {
      expect(screen.getByText('Role')).toBeInTheDocument();
    });

    it('renders a checkbox field', () => {
      expect(screen.getByRole('checkbox', { name: 'Read' })).toBeInTheDocument();
    });

    it('renders a radio field', () => {
      expect(screen.getByRole('radio', { name: 'Small' })).toBeInTheDocument();
    });

    it('renders a date field', () => {
      expect(screen.getByText('Start Date')).toBeInTheDocument();
    });

    it('renders a textarea field', () => {
      expect(screen.getByLabelText('Notes')).toBeInTheDocument();
    });

    it('renders a url field', () => {
      expect(screen.getByText('No URL')).toBeInTheDocument();
    });

    it('renders a map field', () => {
      expect(screen.getByText('Tags')).toBeInTheDocument();
    });

    it('renders a markdown field', () => {
      expect(screen.getByLabelText('Content')).toBeInTheDocument();
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
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
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      expect(screen.getByLabelText('Name')).toHaveValue('Pre-filled');
    });
  });

  describe('validation', () => {
    it('shows minLength error after field is touched', async () => {
      const user = userEvent.setup();

      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name', validation: { minLength: 5 } },
        },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      await user.type(screen.getByLabelText('Name'), 'abc');
      await user.tab();

      await waitFor(() => {
        expect(screen.getByText('Must be at least 5 characters.')).toBeInTheDocument();
      });
    });

    it('shows maxLength error after field is touched', async () => {
      const user = userEvent.setup();

      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name', validation: { maxLength: 3 } },
        },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      await user.type(screen.getByLabelText('Name'), 'toolong');
      await user.tab();

      await waitFor(() => {
        expect(screen.getByText('Must be at most 3 characters.')).toBeInTheDocument();
      });
    });

    it('clears error when value becomes valid', async () => {
      const user = userEvent.setup();

      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name', validation: { minLength: 3 } },
        },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      const input = screen.getByLabelText('Name');
      await user.type(input, 'ab');
      await user.tab();

      await waitFor(() => {
        expect(screen.getByText('Must be at least 3 characters.')).toBeInTheDocument();
      });

      await user.clear(input);
      await user.type(input, 'valid');

      await waitFor(() => {
        expect(screen.queryByText('Must be at least 3 characters.')).not.toBeInTheDocument();
      });
    });

    it('supports custom validate function', async () => {
      const user = userEvent.setup();

      const config: FormConfig = {
        fields: {
          name: {
            type: 'string',
            label: 'Name',
            validation: {
              validate: (value: unknown) => (value === 'bad' ? 'Invalid value' : undefined),
            },
          },
        },
        layout: ['name'],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      await user.type(screen.getByLabelText('Name'), 'bad');
      await user.tab();

      await waitFor(() => {
        expect(screen.getByText('Invalid value')).toBeInTheDocument();
      });
    });
  });

  describe('interpolation', () => {
    it('hides fields when condition layout has when: false', () => {
      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name' },
          secret: { type: 'string', label: 'Secret' },
        },
        layout: ['name', { type: 'condition', when: false, items: ['secret'] }],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      expect(screen.getByLabelText('Name')).toBeInTheDocument();
      expect(screen.queryByLabelText('Secret')).not.toBeInTheDocument();
    });

    it('shows fields when condition layout has when: true', () => {
      const config: FormConfig = {
        fields: {
          name: { type: 'string', label: 'Name' },
          secret: { type: 'string', label: 'Secret' },
        },
        layout: ['name', { type: 'condition', when: true, items: ['secret'] }],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      expect(screen.getByLabelText('Name')).toBeInTheDocument();
      expect(screen.getByLabelText('Secret')).toBeInTheDocument();
    });

    it('reactively shows fields when form value satisfies condition', async () => {
      const user = userEvent.setup();

      const config: FormConfigSchema = {
        fields: {
          showAdvanced: { type: 'boolean', label: 'Advanced', checkboxLabel: 'Show advanced' },
          advanced: { type: 'string', label: 'Advanced Setting' },
        },
        layout: [
          'showAdvanced',
          {
            type: 'condition',
            when: interpolate<boolean, 'form'>(
              ({ page }) => (page as Record<string, unknown>).showAdvanced === true
            ),
            items: ['advanced'],
          },
        ],
      };

      render(
        <ConfigDrivenForm config={config} onSubmit={vi.fn()} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getRouterWrapper(),
          getInterpolationProviderWrapper(),
        ])
      );

      expect(screen.queryByLabelText('Advanced Setting')).not.toBeInTheDocument();

      await user.click(screen.getByRole('checkbox', { name: 'Show advanced' }));

      await waitFor(() => {
        expect(screen.getByLabelText('Advanced Setting')).toBeInTheDocument();
      });
    });
  });
});
