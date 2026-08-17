import { useEffect, useState } from 'react';

import type { DevProfile, GithubUserResponse, UseDevProfileResult } from './types';

const DEV_PROFILE_STORAGE_KEY = 'ma-dev-github-profile';
const DEFAULT_EMAIL = 'dev@localhost';

// Reads a sandbox-only identity override, sourced from a GitHub username passed via the URL
// (?ghUser=<username>, optionally with &email=<email> to override the email directly) or,
// failing that, from localStorage where a previous visit's fetch was cached. The name/avatar are
// derived from a public GitHub profile lookup (https://api.github.com/users/<username>, no auth
// needed); it's all cached together, so setting or changing it never requires rebuilding this
// app: visit once with the query params, then a plain reload keeps showing it. Falls back to
// `undefined` (the existing "Local Developer" / "dev@localhost" / default avatar) when nothing
// has been set — no-op unless a developer opts in.
export function useDevProfile(): UseDevProfileResult {
  const [profile, setProfile] = useState(readCachedDevProfile);
  const [loading, setLoading] = useState(() => isStaleForCurrentUrl(profile));

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const username = params.get('ghUser');
    if (!username) return;
    const emailOverride = params.get('email') ?? undefined;
    // Skips the fetch when the cache already matches: without this, every reload of a URL that
    // still carries ?ghUser=... re-fetches and re-sets state to an equivalent-but-new object,
    // which was causing a visible re-render/flicker of the nav bar on every refresh.
    const cached = readCachedDevProfile();
    if (cached?.username === username && cached.emailOverride === emailOverride) return;
    setLoading(true);
    fetchDevProfile(username, emailOverride)
      .then(setProfile)
      .catch(() => undefined)
      .finally(() => setLoading(false));
  }, []);

  return { ...(profile ?? {}), loading };
}

function isStaleForCurrentUrl(profile: DevProfile | undefined): boolean {
  if (typeof window === 'undefined') return false;
  const params = new URLSearchParams(window.location.search);
  const username = params.get('ghUser');
  if (!username) return false;
  if (profile?.username !== username) return true;
  return profile.emailOverride !== (params.get('email') ?? undefined);
}

function readCachedDevProfile(): DevProfile | undefined {
  if (typeof window === 'undefined') return undefined;
  const raw = window.localStorage.getItem(DEV_PROFILE_STORAGE_KEY);
  if (!raw) return undefined;
  try {
    // cast: value was written by fetchDevProfile below
    return JSON.parse(raw) as DevProfile;
  } catch {
    return undefined;
  }
}

async function fetchDevProfile(username: string, emailOverride?: string): Promise<DevProfile> {
  const response = await fetch(`https://api.github.com/users/${username}`);
  // cast: GitHub public user API shape
  const data = (await response.json()) as GithubUserResponse;
  const profile: DevProfile = {
    username,
    emailOverride,
    name: data.name ?? username,
    email: emailOverride ?? data.email ?? (await fetchLocalGitEmail()) ?? DEFAULT_EMAIL,
    avatarUrl: data.avatar_url,
  };
  window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, JSON.stringify(profile));
  return profile;
}

// GitHub's public API only returns `email` when the user has opted in to a public profile email,
// which most developers haven't, and its noreply fallback address isn't worth showing. When
// running against the local Vite dev server (see the `dev-git-email` plugin in
// app/vite.config.ts), this asks it for `git config user.email` instead, which is usually the
// developer's real address already. That plugin doesn't exist in the built sandbox bundle (nginx
// serves static files there, with no backend to ask), so this just 404s there — the sandbox
// relies on the explicit `&email=` override or the plain default instead.
async function fetchLocalGitEmail(): Promise<string | undefined> {
  try {
    const response = await fetch('/__dev/git-email');
    if (!response.ok) return undefined;
    const email = (await response.text()).trim();
    return email || undefined;
  } catch {
    return undefined;
  }
}

// Clears the cached override and, if the page was loaded with ?ghUser=... still in the URL,
// strips it (and any &email=...) before reloading — otherwise the useEffect in useDevProfile
// would immediately re-fetch and re-cache the same profile, making the clear look like a no-op.
export function clearDevProfile(): void {
  window.localStorage.removeItem(DEV_PROFILE_STORAGE_KEY);
  const url = new URL(window.location.href);
  if (url.searchParams.has('ghUser') || url.searchParams.has('email')) {
    url.searchParams.delete('ghUser');
    url.searchParams.delete('email');
    window.location.assign(url.toString());
    return;
  }
  window.location.reload();
}

export const testing = { DEV_PROFILE_STORAGE_KEY, readCachedDevProfile, fetchDevProfile };
