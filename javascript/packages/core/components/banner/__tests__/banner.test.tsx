import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { Banner } from '../banner';

describe('Banner', () => {
  it('renders children', () => {
    render(<Banner>Pipeline saved successfully</Banner>, buildWrapper([getBaseProviderWrapper()]));

    expect(screen.getByText('Pipeline saved successfully')).toBeInTheDocument();
  });

  it('has complementary role', () => {
    render(<Banner>Content</Banner>, buildWrapper([getBaseProviderWrapper()]));

    expect(screen.getByRole('complementary')).toBeInTheDocument();
  });

  it('renders with title', () => {
    render(
      <Banner title="Warning">Check your configuration</Banner>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Warning')).toBeInTheDocument();
    expect(screen.getByText('Check your configuration')).toBeInTheDocument();
  });

  it('allows overrides to be merged with defaults', () => {
    render(
      <Banner
        overrides={{
          Root: {
            props: { 'data-testid': 'custom-banner' },
          },
        }}
      >
        Content
      </Banner>,
      buildWrapper([getBaseProviderWrapper()])
    );

    const banner = screen.getByTestId('custom-banner');
    expect(banner).toHaveAttribute('role', 'complementary');
  });

  it('preserves default margin reset when consumer provides Root overrides', () => {
    render(
      <Banner
        overrides={{
          Root: {
            style: { padding: '10px' },
            props: { 'data-testid': 'custom-root' },
          },
        }}
      >
        Content
      </Banner>,
      buildWrapper([getBaseProviderWrapper()])
    );

    const banner = screen.getByTestId('custom-root');
    expect(banner).toHaveStyle({
      marginTop: '0px',
      marginRight: '0px',
      marginBottom: '0px',
      marginLeft: '0px',
      padding: '10px',
    });
  });

  it('resets margins on root element', () => {
    render(<Banner>Content</Banner>, buildWrapper([getBaseProviderWrapper()]));

    const banner = screen.getByRole('complementary');
    expect(banner).toHaveStyle({
      marginTop: '0px',
      marginRight: '0px',
      marginBottom: '0px',
      marginLeft: '0px',
    });
  });
});
