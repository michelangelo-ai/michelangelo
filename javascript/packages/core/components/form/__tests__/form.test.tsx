import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { StringField } from '#core/components/form/fields/string/string-field';
import { Form } from '#core/components/form/form';
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
