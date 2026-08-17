import { testing } from '../dev-profile';

const { DEV_PROFILE_STORAGE_KEY, readCachedDevProfile, fetchDevProfile } = testing;

afterEach(() => {
  window.localStorage.clear();
  vi.unstubAllGlobals();
});

function stubFetch(githubResponse: unknown, gitEmailResponse?: { ok: boolean; text?: string }) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      url === '/__dev/git-email'
        ? Promise.resolve({
            ok: gitEmailResponse?.ok ?? false,
            text: () => Promise.resolve(gitEmailResponse?.text ?? ''),
          })
        : Promise.resolve({ json: () => Promise.resolve(githubResponse) })
    )
  );
}

it('returns undefined for missing or corrupted cache instead of throwing', () => {
  expect(readCachedDevProfile()).toBeUndefined();

  window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, 'not json');
  expect(readCachedDevProfile()).toBeUndefined();
});

it('picks an email in priority order: override > GitHub > local git config > default', async () => {
  stubFetch(
    { name: 'Ada Lovelace', email: 'public@example.com', avatar_url: 'https://example.com/a.png' },
    { ok: true, text: 'ada@real-address.dev' }
  );
  expect((await fetchDevProfile('ada', 'override@example.com')).email).toBe('override@example.com');
  expect((await fetchDevProfile('ada')).email).toBe('public@example.com');

  stubFetch(
    { name: null, email: null, avatar_url: 'https://example.com/a.png' },
    { ok: true, text: 'ada@real-address.dev' }
  );
  expect((await fetchDevProfile('ada')).email).toBe('ada@real-address.dev');

  stubFetch({ name: null, email: null, avatar_url: 'https://example.com/a.png' }, { ok: false });
  expect((await fetchDevProfile('ada')).email).toBe('dev@localhost');
});

it('caches the fetched profile', async () => {
  stubFetch({
    name: 'Ada Lovelace',
    email: 'ada@example.com',
    avatar_url: 'https://example.com/a.png',
  });

  const profile = await fetchDevProfile('ada');

  expect(readCachedDevProfile()).toEqual(profile);
});
