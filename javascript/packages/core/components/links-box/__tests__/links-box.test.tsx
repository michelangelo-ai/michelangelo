import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { LinksBox } from '../links-box';

describe('LinksBox', () => {
  it('renders links with name and url', () => {
    render(
      <LinksBox
        links={[
          { name: 'Dashboard', url: 'https://grafana.example.com/d/abc' },
          { name: 'Logs', url: 'https://logs.example.com/app' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper()])
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
        links={[
          { name: 'Valid', url: 'https://example.com' },
          { name: undefined, url: 'https://no-name.com' },
          { name: 'No URL', url: undefined },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('link', { name: 'Valid' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'No URL' })).not.toBeInTheDocument();
    expect(screen.queryByText('no-name.com')).not.toBeInTheDocument();
  });

  it('renders an empty box when all links are incomplete', () => {
    render(
      <LinksBox links={[{ name: undefined, url: undefined }]} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Useful links')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });
});
