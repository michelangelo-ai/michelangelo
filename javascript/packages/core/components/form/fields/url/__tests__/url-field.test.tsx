import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { UrlField } from '#core/components/form/fields/url/url-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('UrlField', () => {
  it('has empty string value on initial render with no initialValue', () => {
    render(
      <UrlField name="website" label="Website" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Website' })).toHaveValue('');
  });

  it('renders input with label', () => {
    render(
      <UrlField name="website" label="Website" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Website' })).toBeInTheDocument();
  });

  it('shows URL validation error on invalid input', async () => {
    const user = userEvent.setup();

    render(
      <UrlField name="website" label="Website" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const input = screen.getByRole('textbox', { name: 'Website' });
    await user.type(input, 'not-a-url');
    await user.tab();

    expect(await screen.findByText('Must be a valid URL.')).toBeInTheDocument();
  });

  it('accepts valid URL without error', async () => {
    const user = userEvent.setup();

    render(
      <UrlField name="website" label="Website" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const input = screen.getByRole('textbox', { name: 'Website' });
    await user.type(input, 'https://example.com');
    await user.tab();

    expect(screen.queryByText('Must be a valid URL.')).not.toBeInTheDocument();
  });
});
