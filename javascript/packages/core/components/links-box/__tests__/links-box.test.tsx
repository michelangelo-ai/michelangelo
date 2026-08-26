import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { LinksBox } from '../links-box';

describe('LinksBox', () => {
  it('renders links with name and url', () => {
    render(
      <LinksBox
        title="Useful links"
        links={[
          { name: 'Dashboard', url: 'https://grafana.example.com/d/abc' },
          { name: 'Logs', url: 'https://logs.example.com/app' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByText('Useful links')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute(
      'href',
      'https://grafana.example.com/d/abc'
    );
    expect(screen.getByRole('link', { name: 'Logs' })).toHaveAttribute(
      'href',
      'https://logs.example.com/app'
    );
  });

  it('filters out links missing a name or url', () => {
    render(
      <LinksBox
        title="Useful links"
        links={[
          { name: 'Valid', url: 'https://example.com' },
          { name: undefined, url: 'https://no-name.com' },
          { name: 'No URL', url: undefined },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByRole('link', { name: 'Valid' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'No URL' })).not.toBeInTheDocument();
    expect(screen.queryByText('no-name.com')).not.toBeInTheDocument();
  });

  it('renders a custom title when provided', () => {
    render(
      <LinksBox
        title="Related dashboards"
        links={[{ name: 'Valid', url: 'https://example.com' }]}
      />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByText('Related dashboards')).toBeInTheDocument();
    expect(screen.queryByText('Useful links')).not.toBeInTheDocument();
  });

  it('renders an empty box when all links are incomplete', () => {
    render(
      <LinksBox title="Useful links" links={[{ name: undefined, url: undefined }]} />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByText('Useful links')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });
});
