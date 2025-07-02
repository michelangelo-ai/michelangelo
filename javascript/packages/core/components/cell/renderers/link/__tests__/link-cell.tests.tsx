import { render, screen } from '@testing-library/react';

import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { LinkCell } from '../link-cell';

describe('LinkCell', () => {
  it('should render text without link when no URL is provided', () => {
    render(
      <LinkCell
        column={{ id: 'spec.link', url: '' }}
        record={{ spec: { link: 'Click me' } }}
        value="Click me"
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getByText('Click me')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('should render text with link when URL is provided', () => {
    render(
      <LinkCell
        column={{ id: 'spec.link', url: 'https://example.com' }}
        record={{ spec: { link: 'Click me' } }}
        value="Click me"
      />,
      { wrapper: getIconProviderWrapper() }
    );

    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', 'https://example.com');
    expect(link).toHaveTextContent('Click me');
  });

  it('should render icon when provided', () => {
    render(
      <LinkCell
        column={{ id: 'spec.link', url: 'https://example.com', icon: 'check' }}
        record={{ spec: { link: 'Click me' } }}
        value="Click me"
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getAllByTitle('Check').length).toBeGreaterThan(0);
  });

  it('should render empty value correctly', () => {
    render(
      <LinkCell
        column={{ id: 'spec.link', url: 'https://example.com' }}
        record={{ spec: { link: '' } }}
        value=""
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getByRole('link')).toHaveTextContent('');
  });
});
