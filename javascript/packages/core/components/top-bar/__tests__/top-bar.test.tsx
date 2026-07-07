import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { TopBar } from '../top-bar';

describe('TopBar', () => {
  let openSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
  });

  afterEach(() => {
    openSpy.mockRestore();
  });

  it('renders the app title as a link to /', async () => {
    render(<TopBar />, buildWrapper([getRouterWrapper(), getBaseProviderWrapper()]));

    // AppNavBar renders both a desktop and mobile title, toggled by CSS media queries jsdom
    // can't evaluate — query with `hidden: true` so the responsive duplicate isn't excluded.
    const titles = await screen.findAllByRole('link', {
      name: 'Michelangelo Studio',
      hidden: true,
    });
    expect(titles.length).toBeGreaterThan(0);
    titles.forEach((title) => expect(title).toHaveAttribute('href', '/'));
  });

  it('opens the default Docs/Help URLs when no links are provided', async () => {
    const user = userEvent.setup();
    render(<TopBar />, buildWrapper([getRouterWrapper(), getBaseProviderWrapper()]));

    await user.click(screen.getByText('Docs'));
    expect(openSpy).toHaveBeenCalledWith(
      'https://michelangelo-ai.org/docs/',
      '_blank',
      'noopener,noreferrer'
    );

    await user.click(screen.getByText('Help'));
    expect(openSpy).toHaveBeenCalledWith(
      'https://michelangelo-ai.org/docs/#support--community',
      '_blank',
      'noopener,noreferrer'
    );
  });

  it('replaces the defaults entirely when adopter links are provided', async () => {
    const user = userEvent.setup();
    const links = [{ label: 'Support', url: 'https://adopter.example.com/support' }];
    render(<TopBar links={links} />, buildWrapper([getRouterWrapper(), getBaseProviderWrapper()]));

    expect(screen.queryByText('Docs')).not.toBeInTheDocument();

    await user.click(screen.getByText('Support'));
    expect(openSpy).toHaveBeenCalledWith(
      'https://adopter.example.com/support',
      '_blank',
      'noopener,noreferrer'
    );
  });
});
