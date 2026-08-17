import { clearDevProfile, testing } from '../dev-profile';

const { DEV_PROFILE_STORAGE_KEY, readCachedDevProfile, fetchDevProfile } = testing;

describe('readCachedDevProfile', () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  it('returns undefined when nothing is cached', () => {
    expect(readCachedDevProfile()).toBeUndefined();
  });

  it('returns the cached profile', () => {
    const profile = {
      username: 'ada',
      name: 'Ada',
      email: 'ada@example.com',
      avatarUrl: 'https://example.com/a.png',
    };
    window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, JSON.stringify(profile));

    expect(readCachedDevProfile()).toEqual(profile);
  });

  it('returns undefined for corrupted cache instead of throwing', () => {
    window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, 'not json');

    expect(readCachedDevProfile()).toBeUndefined();
  });
});

describe('fetchDevProfile', () => {
  afterEach(() => {
    window.localStorage.clear();
    vi.unstubAllGlobals();
  });

  function stubFetch(githubResponse: unknown, gitEmailResponse?: { ok: boolean; text?: string }) {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url === '/__dev/git-email') {
          return Promise.resolve({
            ok: gitEmailResponse?.ok ?? false,
            text: () => Promise.resolve(gitEmailResponse?.text ?? ''),
          });
        }
        return Promise.resolve({ json: () => Promise.resolve(githubResponse) });
      })
    );
  }

  it('maps the GitHub response to a dev profile and caches it', async () => {
    stubFetch({
      name: 'Ada Lovelace',
      email: 'ada@example.com',
      avatar_url: 'https://example.com/a.png',
    });

    const profile = await fetchDevProfile('ada');

    expect(profile).toEqual({
      username: 'ada',
      name: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: 'https://example.com/a.png',
    });
    expect(readCachedDevProfile()).toEqual(profile);
  });

  it('an explicit email override wins over both GitHub and the local git config', async () => {
    stubFetch(
      {
        name: 'Ada Lovelace',
        email: 'public@example.com',
        avatar_url: 'https://example.com/a.png',
      },
      { ok: true, text: 'ada@real-address.dev' }
    );

    const profile = await fetchDevProfile('ada', 'override@example.com');

    expect(profile.email).toBe('override@example.com');
    expect(profile.emailOverride).toBe('override@example.com');
  });

  it('falls back to the username and the local git email when GitHub has no public email', async () => {
    stubFetch(
      { name: null, email: null, avatar_url: 'https://example.com/a.png' },
      { ok: true, text: 'ada@real-address.dev' }
    );

    const profile = await fetchDevProfile('ada');

    expect(profile.name).toBe('ada');
    expect(profile.email).toBe('ada@real-address.dev');
  });

  it('falls back to the plain default email when neither GitHub nor the local git config has one', async () => {
    stubFetch({ name: null, email: null, avatar_url: 'https://example.com/a.png' }, { ok: false });

    const profile = await fetchDevProfile('ada');

    expect(profile.email).toBe('dev@localhost');
  });
});

describe('clearDevProfile', () => {
  const profile = {
    username: 'ada',
    name: 'Ada',
    email: 'ada@example.com',
    avatarUrl: 'https://example.com/a.png',
  };
  const originalLocation = window.location;

  // jsdom's window.location.assign/reload are non-configurable, so vi.spyOn can't stub them
  // directly; swap the whole object for a plain copy with the method under test replaced.
  function stubLocation(overrides: Partial<Location>): void {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { ...window.location, ...overrides },
    });
  }

  afterEach(() => {
    window.localStorage.clear();
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation });
    window.history.replaceState(null, '', '/');
    vi.restoreAllMocks();
  });

  it('removes the cached profile and strips ghUser from the URL before reloading', () => {
    window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, JSON.stringify(profile));
    window.history.replaceState(null, '', '/?ghUser=ada');
    const assign = vi.fn();
    stubLocation({ assign });

    clearDevProfile();

    expect(readCachedDevProfile()).toBeUndefined();
    expect(assign).toHaveBeenCalledTimes(1);
    expect(assign.mock.calls[0][0]).not.toContain('ghUser');
  });

  it('reloads in place when the URL has no ghUser param to strip', () => {
    window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, JSON.stringify(profile));
    window.history.replaceState(null, '', '/');
    const reload = vi.fn();
    stubLocation({ reload });

    clearDevProfile();

    expect(readCachedDevProfile()).toBeUndefined();
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
