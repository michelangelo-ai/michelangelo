import { renderHook, waitFor } from '@testing-library/react';

import {
  clearDevProfile,
  DEV_PROFILE_STORAGE_KEY,
  EMAIL_PARAM,
  GH_USER_PARAM,
  useDevProfile,
} from '../dev-profile';

afterEach(() => {
  window.localStorage.clear();
  vi.unstubAllGlobals();
  window.history.replaceState(null, '', '/');
});

function setUrlParams(params: Record<string, string>) {
  window.history.replaceState(null, '', `/?${new URLSearchParams(params).toString()}`);
}

function stubFetch(
  githubResponse: unknown,
  gitEmailResponse?: { ok: boolean; text?: string; contentType?: string }
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      url === '/__dev/git-email'
        ? Promise.resolve({
            ok: gitEmailResponse?.ok ?? false,
            headers: { get: () => gitEmailResponse?.contentType ?? 'text/plain' },
            text: () => Promise.resolve(gitEmailResponse?.text ?? ''),
          })
        : Promise.resolve({ json: () => Promise.resolve(githubResponse) })
    )
  );
}

it('does not fetch or show a profile when the URL has no ?ghUser=', () => {
  const { result } = renderHook(() => useDevProfile());

  expect(result.current.loading).toBe(false);
  expect(result.current.username).toBeUndefined();
});

it('picks an email in priority order: override > GitHub > local git config > default', async () => {
  setUrlParams({ [GH_USER_PARAM]: 'ada', [EMAIL_PARAM]: 'override@example.com' });
  stubFetch(
    { name: 'Ada Lovelace', email: 'public@example.com', avatar_url: 'https://example.com/a.png' },
    { ok: true, text: 'ada@real-address.dev' }
  );
  const override = renderHook(() => useDevProfile());
  await waitFor(() => expect(override.result.current.loading).toBe(false));
  expect(override.result.current.email).toBe('override@example.com');

  window.localStorage.clear();
  setUrlParams({ [GH_USER_PARAM]: 'ada' });
  const githubEmail = renderHook(() => useDevProfile());
  await waitFor(() => expect(githubEmail.result.current.loading).toBe(false));
  expect(githubEmail.result.current.email).toBe('public@example.com');

  window.localStorage.clear();
  stubFetch(
    { name: null, email: null, avatar_url: 'https://example.com/a.png' },
    { ok: true, text: 'ada@real-address.dev' }
  );
  const localGitEmail = renderHook(() => useDevProfile());
  await waitFor(() => expect(localGitEmail.result.current.loading).toBe(false));
  expect(localGitEmail.result.current.email).toBe('ada@real-address.dev');

  window.localStorage.clear();
  stubFetch({ name: null, email: null, avatar_url: 'https://example.com/a.png' }, { ok: false });
  const defaultEmail = renderHook(() => useDevProfile());
  await waitFor(() => expect(defaultEmail.result.current.loading).toBe(false));
  expect(defaultEmail.result.current.email).toBe('dev@localhost');
});

it('falls through to the default email when /__dev/git-email returns nginx SPA-fallback HTML', async () => {
  setUrlParams({ [GH_USER_PARAM]: 'ada' });
  stubFetch(
    { name: null, email: null, avatar_url: 'https://example.com/a.png' },
    {
      ok: true,
      text: '<!doctype html><html><head></head><body></body></html>',
      contentType: 'text/html',
    }
  );

  const { result } = renderHook(() => useDevProfile());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.email).toBe('dev@localhost');
});

it('caches the fetched profile', async () => {
  setUrlParams({ [GH_USER_PARAM]: 'ada' });
  stubFetch({
    name: 'Ada Lovelace',
    email: 'ada@example.com',
    avatar_url: 'https://example.com/a.png',
  });

  const { result } = renderHook(() => useDevProfile());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(JSON.parse(window.localStorage.getItem(DEV_PROFILE_STORAGE_KEY) ?? '')).toEqual({
    username: 'ada',
    emailOverride: undefined,
    name: 'Ada Lovelace',
    email: 'ada@example.com',
    avatarUrl: 'https://example.com/a.png',
  });
});

it('skips refetching when the cache already matches the current URL', () => {
  setUrlParams({ [GH_USER_PARAM]: 'ada' });
  window.localStorage.setItem(
    DEV_PROFILE_STORAGE_KEY,
    JSON.stringify({
      username: 'ada',
      name: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: 'https://example.com/a.png',
    })
  );
  const fetchSpy = vi.fn();
  vi.stubGlobal('fetch', fetchSpy);

  const { result } = renderHook(() => useDevProfile());

  expect(result.current.loading).toBe(false);
  expect(result.current.name).toBe('Ada Lovelace');
  expect(fetchSpy).not.toHaveBeenCalled();
});

it('clears the cached profile', () => {
  window.localStorage.setItem(
    DEV_PROFILE_STORAGE_KEY,
    JSON.stringify({ username: 'ada', name: 'Ada Lovelace' })
  );
  clearDevProfile();

  expect(window.localStorage.getItem(DEV_PROFILE_STORAGE_KEY)).toBeNull();
});
