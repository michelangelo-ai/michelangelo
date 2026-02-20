import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { App } from '../App';
import { DEV_PROFILE_STORAGE_KEY, GH_USER_PARAM } from '../dev-profile';

vi.mock('@michelangelo-ai/rpc', () => ({
  request: vi.fn((queryName: string) =>
    queryName === 'ListProject'
      ? Promise.resolve({ projectList: { items: [] } })
      : Promise.reject(new Error(`Unexpected query: ${queryName}`))
  ),
  normalizeTranscoderError: vi.fn(() => null),
}));

// AppNavBar renders both its desktop and mobile menu variants at once. The inactive variant
// reports as hidden to the accessibility tree, so role queries need `hidden: true`.
const HIDDEN = { hidden: true };

afterEach(() => {
  window.localStorage.clear();
  vi.unstubAllGlobals();
  window.history.replaceState(null, '', '/');
});

it('shows the GitHub profile fetched via ?ghUser= in the nav bar', async () => {
  window.history.replaceState(null, '', `/?${new URLSearchParams({ [GH_USER_PARAM]: 'ada' })}`);
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        json: () =>
          Promise.resolve({
            name: 'Ada Lovelace',
            email: 'ada@example.com',
            avatar_url: 'https://example.com/a.png',
          }),
      })
    )
  );

  render(<App />);

  expect(
    (await screen.findAllByRole('button', { name: /Ada Lovelace/, ...HIDDEN })).length
  ).toBeGreaterThan(0);
});

it('signs out to clear the cached dev profile', async () => {
  window.localStorage.setItem(
    DEV_PROFILE_STORAGE_KEY,
    JSON.stringify({
      username: 'ada',
      name: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: 'https://example.com/a.png',
    })
  );
  render(<App />);
  await userEvent.click(
    (await screen.findAllByRole('button', { name: /Ada Lovelace/, ...HIDDEN }))[0]
  );
  await userEvent.click(screen.getByRole('option', { name: 'Sign out' }));

  expect(window.localStorage.getItem(DEV_PROFILE_STORAGE_KEY)).toBeNull();
});
