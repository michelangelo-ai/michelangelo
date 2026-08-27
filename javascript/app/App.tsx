import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { CoreApp, TimeZone, UserRole } from '@michelangelo-ai/core';
import { normalizeTranscoderError, request } from '@michelangelo-ai/rpc';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { clearDevProfile, useDevProfile } from './dev-profile';
import { ICONS } from './icons/icons';

// Sandbox/demo app shell; a real deployment supplies its own identity via a different shell.
const engine = new Styletron();
const queryClient = new QueryClient();

// Sandbox/demo-only: fakes Tier-2 ownership resolution (see packages/core's Owner column
// config) by returning a fixed team for any UUID, rather than wiring a real resolver backend.
const FAKE_TEAM = {
  id: 'michelangelo',
  displayName: 'Michelangelo',
  url: 'https://github.com/michelangelo-ai/michelangelo',
};

function resolveTeams(uuids: string[]) {
  return Promise.resolve(Object.fromEntries(uuids.map((uuid) => [uuid, FAKE_TEAM])));
}

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
      resolvers: {
        team: resolveTeams,
      },
    },
    navigationBar: {
      links: [{ label: 'Docs', href: 'https://michelangelo-ai.github.io/michelangelo/' }],
      onSignOut: clearDevProfile,
    },
    user: {
      name: devProfile.name ?? (devProfile.loading ? undefined : 'Local Developer'),
      email: devProfile.email ?? (devProfile.loading ? undefined : 'dev@localhost'),
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
