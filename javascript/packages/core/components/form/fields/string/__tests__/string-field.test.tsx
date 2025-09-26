import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { StringField } from '#core/components/form/fields/string/string-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('StringField', () => {
  it('renders with label', () => {
    render(
      <StringField name="email" label="Email Address" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Email Address' })).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <StringField name="email" label="Email" required />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Email *' })).toBeInTheDocument();
  });

  it('handles user input', async () => {
    const user = userEvent.setup();

    render(
      <StringField name="email" label="Email" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const input = screen.getByRole('textbox', { name: 'Email' });
    await user.type(input, 'test@example.com');

    expect(input).toHaveValue('test@example.com');
  });

  it('displays help tooltip when description is provided', () => {
    render(
      <StringField name="email" label="Email" description="Your email address for notifications" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper()])
    );

    expect(screen.getByRole('img', { name: 'help' })).toBeInTheDocument();
  });
});
