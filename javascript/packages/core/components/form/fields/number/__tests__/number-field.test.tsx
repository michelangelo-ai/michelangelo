import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { NumberField } from '#core/components/form/fields/number/number-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('NumberField', () => {
  it('has empty string value on initial render with no initialValue', () => {
    render(
      <NumberField name="count" label="Count" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('spinbutton', { name: 'Count' })).toHaveValue(null);
  });

  it('renders with label', () => {
    render(
      <NumberField name="count" label="Count" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('spinbutton', { name: 'Count' })).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <NumberField name="count" label="Count" required />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('spinbutton', { name: 'Count *' })).toBeInTheDocument();
  });

  it('handles numeric input', async () => {
    const user = userEvent.setup();

    render(
      <NumberField name="count" label="Count" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const input = screen.getByRole('spinbutton', { name: 'Count' });
    await user.type(input, '42');

    expect(input).toHaveValue(42);
  });
});
