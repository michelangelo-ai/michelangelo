import { useEffect, useState } from 'react';
import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { CoreApp, TimeZone, UserRole } from '@michelangelo-ai/core';
import { normalizeTranscoderError, request } from '@michelangelo-ai/rpc';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { ICONS } from './icons/icons';

import type { DevProfile, GithubUserResponse } from './types';

const DEV_PROFILE_STORAGE_KEY = 'ma-dev-github-profile';

// Reads a sandbox-only identity override, sourced from a GitHub username passed via the URL
// (?ghUser=<username>) or, failing that, from localStorage where a previous visit's fetch was
// cached. The name/email/avatar are all derived from one public GitHub profile lookup
// (https://api.github.com/users/<username>, no auth needed) and cached together, so setting or
// changing the username never requires rebuilding this app: visit once with the query param,
// then a plain reload keeps showing it. Falls back to `undefined` (the existing "Local
// Developer" / "dev@localhost" / default avatar) when nothing has been set — no-op unless a
// developer opts in.
//
// This whole file is the sandbox/demo app shell (see the hardcoded fallback name/email/role
// below); a real production deployment would supply its own user identity from its own identity
// service via a different app shell, so this override never reaches production.
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

async function fetchDevProfile(username: string): Promise<DevProfile> {
  const response = await fetch(`https://api.github.com/users/${username}`);
  // cast: GitHub public user API shape
  const data = (await response.json()) as GithubUserResponse;
  const profile: DevProfile = {
    name: data.name ?? username,
    email: data.email ?? `${username}@users.noreply.github.com`,
    avatarUrl: data.avatar_url,
  };
  window.localStorage.setItem(DEV_PROFILE_STORAGE_KEY, JSON.stringify(profile));
  return profile;
}

function useDevProfile(): Partial<DevProfile> {
  const [profile, setProfile] = useState(readCachedDevProfile);

  useEffect(() => {
    const username = new URLSearchParams(window.location.search).get('ghUser');
    if (!username) return;
    fetchDevProfile(username)
      .then(setProfile)
      .catch(() => undefined);
  }, []);

  return profile ?? {};
}

const engine = new Styletron();
const queryClient = new QueryClient();

export function App() {
  const devProfile = useDevProfile();

  const dependencies = {
    error: {
      normalizeError: normalizeTranscoderError,
    },
    theme: {
      icons: ICONS,
    },
    service: {
      request,
    },
    navigationBar: {
      links: [{ label: 'Docs', href: 'https://michelangelo-ai.github.io/michelangelo/' }],
    },
    user: {
      name: devProfile.name ?? 'Local Developer',
      email: devProfile.email ?? 'dev@localhost',
      role: UserRole.Admin,
      timeZone: TimeZone.Local,
      avatarUrl: devProfile.avatarUrl,
    },
  };

  return (
    <StyletronProvider value={engine}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
            <Route path="/*" element={<CoreApp dependencies={dependencies} />} />
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </StyletronProvider>
  );
}
