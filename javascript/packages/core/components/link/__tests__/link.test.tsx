import { render, screen } from '@testing-library/react';
import { Alert } from 'baseui/icon';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { Link } from '../link';

import type { ReactNode } from 'react';

describe('Link', () => {
  describe('internal link', () => {
    let link: HTMLElement;
    beforeEach(async () => {
      render(
        <Link href="/internal-path" title="Internal Link">
          Click me
        </Link>,
        buildWrapper([getRouterWrapper(), getIconProviderWrapper()])
      );

      link = await screen.findByRole('link', { name: 'Click me' });
    });

    it('should render a link element', () => {
      expect(link).toBeInTheDocument();
    });

    it('should have the correct href attribute', () => {
      expect(link).toHaveAttribute('href', '/internal-path');
    });

    it('should have the correct title attribute', () => {
      expect(link).toHaveAttribute('title', 'Internal Link');
    });

    it('should not display external link text', () => {
      expect(screen.queryByTitle('External link')).not.toBeInTheDocument();
    });
  });

  describe('external link', () => {
    let link: HTMLElement;
    beforeEach(async () => {
      render(
        <Link href="https://example.com">External Link</Link>,
        buildWrapper([
          getRouterWrapper(),
          getIconProviderWrapper({ icons: { arrowLaunch: Alert } }),
        ])
      );

      link = await screen.findByRole('link', { name: /External Link/ });
    });

    it('should render a link element', () => {
      expect(link).toBeInTheDocument();
    });

    it('should have the correct href attribute', () => {
      expect(link).toHaveAttribute('href', 'https://example.com');
    });

    it('should display external link icon', () => {
      expect(screen.getAllByTitle('External link').length).toBeGreaterThan(0);
    });
  });

  describe('custom overrides', () => {
    beforeEach(() => {
      const CustomLink = ({ children, ...props }: { children: ReactNode; $external: boolean }) => (
        <a data-testid="custom-link" {...props}>
          {children}
        </a>
      );
      const CustomIcon = (props) => <span data-testid="custom-icon" {...props} />;

      render(
        <Link
          href="https://example.com"
          overrides={{
            Link: { component: CustomLink },
            ExternalLinkIcon: { component: CustomIcon },
          }}
        >
          Custom Link
        </Link>,
        buildWrapper([getRouterWrapper(), getIconProviderWrapper()])
      );
    });

    it('should render custom link component', () => {
      expect(screen.getByTestId('custom-link')).toBeInTheDocument();
    });

    it('should render custom icon component', () => {
      expect(screen.getByTestId('custom-icon')).toBeInTheDocument();
    });
  });
});
