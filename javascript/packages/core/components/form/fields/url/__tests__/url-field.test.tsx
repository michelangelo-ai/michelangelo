import { render, screen } from '@testing-library/react';

import { UrlField } from '#core/components/form/fields/url/url-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('UrlField', () => {
  it('renders a link when value is a valid URL', async () => {
    render(
      <UrlField name="url" label="Website" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { url: 'https://example.com' } }),
      ])
    );

    const link = await screen.findByRole('link');
    expect(link).toHaveAttribute('href', 'https://example.com');
  });

  it('uses urlName as link text when provided', async () => {
    render(
      <UrlField name="url" label="Website" urlName="My Site" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { url: 'https://example.com' } }),
      ])
    );

    expect(await screen.findByText('My Site')).toBeInTheDocument();
  });

  it('shows no link and no raw value when URL is invalid', () => {
    render(
      <UrlField name="url" label="Website" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { url: 'not-a-url' } }),
      ])
    );

    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.queryByText('not-a-url')).not.toBeInTheDocument();
  });

  it('shows placeholder when URL is invalid and placeholder is set', () => {
    render(
      <UrlField name="url" label="Website" placeholder="No URL configured" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { url: 'not-a-url' } }),
      ])
    );

    expect(screen.getByText('No URL configured')).toBeInTheDocument();
  });

  it('uses label as link text when urlName is not provided', async () => {
    render(
      <UrlField name="url" label="Website" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { url: 'https://example.com' } }),
      ])
    );

    const link = await screen.findByRole('link');
    expect(link).toHaveTextContent('Website');
  });

  it('renders label', () => {
    render(
      <UrlField name="url" label="Website" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Website')).toBeInTheDocument();
  });
});
