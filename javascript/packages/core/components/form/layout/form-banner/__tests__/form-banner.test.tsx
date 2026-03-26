import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { FormBanner } from '../form-banner';

const xFilledIcon = () => <svg data-testid="x-filled-icon" />;

describe('FormBanner', () => {
  it('renders content', () => {
    render(
      <FormBanner content="Pipeline will be deprecated soon" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Pipeline will be deprecated soon')).toBeInTheDocument();
  });

  it('renders title when provided', () => {
    render(
      <FormBanner title="Notice" content="Something happened" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Notice')).toBeInTheDocument();
  });

  it('does not show dismiss button by default', () => {
    render(
      <FormBanner content="Informational message" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('can be dismissed when dismissible is true', async () => {
    const user = userEvent.setup();

    render(
      <FormBanner content="Dismissible message" dismissible />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({ icons: { 'x-filled': xFilledIcon } }),
      ])
    );

    expect(screen.getByText('Dismissible message')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Dismiss' }));

    expect(screen.queryByText('Dismissible message')).not.toBeInTheDocument();
  });

  it('renders string content as Markdown', () => {
    render(
      <FormBanner content="This is **bold** text" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('bold').tagName).toBe('STRONG');
  });

  it('renders ReactNode content directly', () => {
    render(
      <FormBanner content={<span data-testid="custom-content">Custom element</span>} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByTestId('custom-content')).toBeInTheDocument();
  });
});
