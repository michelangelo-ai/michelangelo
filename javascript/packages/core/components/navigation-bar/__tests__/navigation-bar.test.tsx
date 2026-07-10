import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { NavigationBar } from '../navigation-bar';

import type { NavigationLink } from '../types';

describe('NavigationBar', () => {
  it('renders the title', () => {
    render(<NavigationBar />, buildWrapper([getBaseProviderWrapper(), getRouterWrapper()]));

    expect(screen.getAllByText('Michelangelo Studio').length).toBeGreaterThan(0);
  });

  it('renders navigation links', () => {
    const links: NavigationLink[] = [
      { label: 'Docs', href: 'https://example.com/docs' },
      { label: 'Help', href: 'https://example.com/help' },
    ];

    render(
      <NavigationBar links={links} />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getAllByText('Docs').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Help').length).toBeGreaterThan(0);
  });

  it('opens link in a new tab when clicked', async () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    const links: NavigationLink[] = [{ label: 'Docs', href: 'https://example.com/docs' }];

    render(
      <NavigationBar links={links} />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper()])
    );
    await userEvent.click(screen.getAllByText('Docs')[0]);

    expect(openSpy).toHaveBeenCalledWith(
      'https://example.com/docs',
      '_blank',
      'noopener,noreferrer'
    );

    openSpy.mockRestore();
  });
});
