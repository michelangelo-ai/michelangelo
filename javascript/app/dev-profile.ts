import { useEffect, useState } from 'react';

import type { DevProfile, GithubUserResponse, UseDevProfileResult } from './types';

export const DEV_PROFILE_STORAGE_KEY = 'ma-dev-github-profile';
export const GH_USER_PARAM = 'ghUser';
export const EMAIL_PARAM = 'email';
const DEFAULT_EMAIL = 'dev@localhost';

// Sandbox-only identity override sourced from ?ghUser=<username>[&email=<email>] in the URL,
// cached to localStorage so it survives reloads without a rebuild.
export function useDevProfile(): UseDevProfileResult {
  const [profile, setProfile] = useState(readCachedDevProfile);
  const [loading, setLoading] = useState(() => isStaleForCurrentUrl(profile));

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const username = params.get(GH_USER_PARAM);
    if (!username) return;
    const emailOverride = params.get(EMAIL_PARAM) ?? undefined;
    // Avoid refetching (and flickering the nav bar) when the cache already matches.
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
  const username = params.get(GH_USER_PARAM);
  if (!username) return false;
  if (profile?.username !== username) return true;
  return profile.emailOverride !== (params.get(EMAIL_PARAM) ?? undefined);
}

function readCachedDevProfile(): DevProfile | undefined {
  if (typeof window === 'undefined') return undefined;
  const raw = window.localStorage.getItem(DEV_PROFILE_STORAGE_KEY);
  if (!raw) return undefined;
  try {
    // cast: written by fetchDevProfile below
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

// Asks the Vite dev server (see the `dev-git-email` plugin in vite.config.ts) for
// `git config user.email`. 404s against the built sandbox bundle, which has no backend to ask.
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

// Strips ?ghUser=/&email= from the URL before reloading, so useDevProfile doesn't immediately
// refetch and re-cache the same profile.
export function clearDevProfile(): void {
  window.localStorage.removeItem(DEV_PROFILE_STORAGE_KEY);
  const url = new URL(window.location.href);
  if (url.searchParams.has(GH_USER_PARAM) || url.searchParams.has(EMAIL_PARAM)) {
    url.searchParams.delete(GH_USER_PARAM);
    url.searchParams.delete(EMAIL_PARAM);
    window.location.assign(url.toString());
    return;
  }
  window.location.reload();
}
