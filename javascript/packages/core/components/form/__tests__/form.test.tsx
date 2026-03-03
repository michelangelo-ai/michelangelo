import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { Form } from '#core/components/form/form';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { minLength, required } from '#core/components/form/validation/validators';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('Form integration', () => {
  it('submits form with multiple field values', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <Form onSubmit={onSubmit}>
        <StringField name="email" label="Email" />
        <StringField name="name" label="Name" />
        <button type="submit">Submit</button>
      </Form>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.type(screen.getByLabelText('Name'), 'John Doe');
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        {
          email: 'test@example.com',
          name: 'John Doe',
        },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('provides initial values to fields', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    const initialValues = { email: 'initial@example.com', name: 'Initial User' };

    render(
      <div>
        <Form onSubmit={onSubmit} initialValues={initialValues}>
          <StringField name="email" label="Email" />
          <StringField name="name" label="Name" />
          <button type="submit">Submit</button>
        </Form>
      </div>,

      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('initial@example.com');
    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Initial User');
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(initialValues, expect.anything(), expect.anything())
    );
  });

  it('supports external submit button via form id', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <div>
        <Form id="test-form" onSubmit={onSubmit}>
          <StringField name="email" label="Email" />
        </Form>
        <button type="submit" form="test-form">
          External Submit
        </button>
      </div>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.click(screen.getByRole('button', { name: 'External Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { email: 'test@example.com' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('supports render prop for wrapping form element', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <Form
        id="wrapped-form"
        onSubmit={onSubmit}
        render={(formElement) => (
          <div data-testid="wrapper">
            <div data-testid="header">Header Content</div>
            {formElement}
            <div data-testid="footer">Footer Content</div>
          </div>
        )}
      >
        <StringField name="email" label="Email" />
        <button type="submit">Submit</button>
      </Form>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByTestId('wrapper')).toBeInTheDocument();
    expect(screen.getByTestId('header')).toHaveTextContent('Header Content');
    expect(screen.getByTestId('footer')).toHaveTextContent('Footer Content');
    expect(screen.getByLabelText('Email')).toBeInTheDocument();

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { email: 'test@example.com' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('allows external submit button in render prop wrapper via form id', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <Form
        id="wrapped-form"
        onSubmit={onSubmit}
        render={(formElement) => (
          <div data-testid="wrapper">
            {formElement}
            <div data-testid="footer">
              <button type="submit" form="wrapped-form">
                External Submit
              </button>
            </div>
          </div>
        )}
      >
        <StringField name="email" label="Email" />
      </Form>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.click(screen.getByRole('button', { name: 'External Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { email: 'test@example.com' },
        expect.anything(),
        expect.anything()
      )
    );
  });
});

describe('Form validation', () => {
  it('allows submission after required field is filled', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <Form onSubmit={onSubmit}>
        <StringField name="username" label="Username" required />
        <button type="submit">Submit</button>
      </Form>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    expect(await screen.findByText('This field is required.')).toBeInTheDocument();

    await user.type(screen.getByRole('textbox', { name: 'Username *' }), 'johndoe');
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { username: 'johndoe' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('shows first error when composed validators fail sequentially', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <Form onSubmit={onSubmit}>
        <StringField
          name="username"
          label="Username"
          required
          validate={combineValidators(required(), minLength(6))}
        />
        <button type="submit">Submit</button>
      </Form>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    expect(await screen.findByText('This field is required.')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();

    await user.type(screen.getByRole('textbox', { name: 'Username *' }), 'abc');
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    expect(await screen.findByText('Must be at least 6 characters.')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe('FormDialog', () => {
  const defaultProps = {
    isOpen: true,
    onDismiss: vi.fn(),
    heading: 'Test Dialog',
    onSubmit: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders dialog with form when open', async () => {
    render(
      <FormDialog {...defaultProps}>
        <StringField name="email" label="Email" />
      </FormDialog>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await screen.findByRole('dialog', { name: 'Test Dialog' });
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('does not render when closed', async () => {
    render(
      <FormDialog {...defaultProps} isOpen={false}>
        <StringField name="email" label="Email" />
      </FormDialog>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    try {
      await screen.findByRole('dialog', {}, { timeout: 100 });
      throw new Error('Dialog should not be in the document');
    } catch (e: unknown) {
      if (e instanceof Error) {
        if (e.name !== 'TestingLibraryElementError') throw e;
      } else {
        throw e;
      }

      // Success!
    }
  });

  it('submits form data and auto-closes on success', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    const onDismiss = vi.fn();

    render(
      <FormDialog {...defaultProps} onSubmit={onSubmit} onDismiss={onDismiss}>
        <StringField name="email" label="Email" />
      </FormDialog>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith({ email: 'test@example.com' });
    });

    await waitFor(() => {
      expect(onDismiss).toHaveBeenCalledTimes(1);
    });
  });

  it('calls onDismiss when cancel is clicked', async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();

    render(
      <FormDialog {...defaultProps} onDismiss={onDismiss}>
        <StringField name="email" label="Email" />
      </FormDialog>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('handles submit errors without auto-closing', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockRejectedValue(new Error('Submit failed'));
    const onDismiss = vi.fn();

    render(
      <FormDialog {...defaultProps} onSubmit={onSubmit} onDismiss={onDismiss}>
        <StringField name="email" label="Email" />
      </FormDialog>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
    });

    expect(onDismiss).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog', { name: 'Test Dialog' })).toBeInTheDocument();
    await screen.findByText(/Submit failed/);
  });

  it('supports custom submit label and initial values', () => {
    render(
      <FormDialog
        {...defaultProps}
        submitLabel="Create Item"
        initialValues={{ email: 'preset@example.com' }}
      >
        <StringField name="email" label="Email" />
      </FormDialog>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByRole('button', { name: 'Create Item' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('preset@example.com');
  });
});
