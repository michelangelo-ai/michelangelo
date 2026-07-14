import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';
import { NavigationBar } from '../navigation-bar';

import type { NavigationLink } from '../types';

describe('NavigationBar', () => {
  // AppNavBar renders both its desktop and mobile menu variants at once and switches
  // between them with a `matchMedia` query, which jsdom doesn't implement — the inactive
  // variant reports as hidden to the accessibility tree, so role queries need `hidden: true`.
  it('renders the title as a link to the app root', () => {
    render(
      <NavigationBar />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper(), getUserProviderWrapper()])
    );

    const titleLinks = screen.getAllByRole('link', {
      name: 'Michelangelo Studio',
      hidden: true,
    });
    expect(titleLinks.length).toBeGreaterThan(0);
    titleLinks.forEach((link) => expect(link).toHaveAttribute('href', '/'));
  });

  it('renders navigation links with their destination href', () => {
    const links: NavigationLink[] = [
      { label: 'Docs', href: 'https://example.com/docs' },
      { label: 'Help', href: 'https://example.com/help' },
    ];

    render(
      <NavigationBar links={links} />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper(), getUserProviderWrapper()])
    );

    const docsLinks = screen.getAllByRole('link', { name: 'Docs', hidden: true });
    expect(docsLinks.length).toBeGreaterThan(0);
    docsLinks.forEach((link) => expect(link).toHaveAttribute('href', 'https://example.com/docs'));

    const helpLinks = screen.getAllByRole('link', { name: 'Help', hidden: true });
    expect(helpLinks.length).toBeGreaterThan(0);
    helpLinks.forEach((link) => expect(link).toHaveAttribute('href', 'https://example.com/help'));
  });

  it('opens navigation links in a new tab without leaking a window reference', () => {
    const links: NavigationLink[] = [{ label: 'Docs', href: 'https://example.com/docs' }];

    render(
      <NavigationBar links={links} />,
      buildWrapper([getBaseProviderWrapper(), getRouterWrapper(), getUserProviderWrapper()])
    );

    const docsLinks = screen.getAllByRole('link', { name: 'Docs', hidden: true });
    expect(docsLinks.length).toBeGreaterThan(0);
    docsLinks.forEach((link) => {
      expect(link).toHaveAttribute('target', '_blank');
      expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
    });
  });

  it('renders user identity from the user provider', () => {
    render(
      <NavigationBar />,
      buildWrapper([
        getBaseProviderWrapper(),
        getRouterWrapper(),
        getUserProviderWrapper({ name: 'Test User', email: 'test@example.com' }),
      ])
    );

    expect(
      screen.getAllByRole('button', { name: /Test User/, hidden: true }).length
    ).toBeGreaterThan(0);
  });

  it('shows the Sign out menu item when the user menu is opened', async () => {
    render(
      <NavigationBar />,
      buildWrapper([
        getBaseProviderWrapper(),
        getRouterWrapper(),
        getUserProviderWrapper({ name: 'Test User', email: 'test@example.com' }),
      ])
    );

    await userEvent.click(screen.getAllByRole('button', { name: /Test User/, hidden: true })[0]);

    expect(screen.getByRole('option', { name: 'Sign out' })).toBeInTheDocument();
  });
});
