import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { CoreApp, TimeZone, UserRole } from '@michelangelo-ai/core';
import { normalizeTranscoderError, request } from '@michelangelo-ai/rpc';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { ICONS } from './icons/icons';

const DEV_USER = {
  name: 'Local Developer',
  email: 'dev@localhost',
  role: UserRole.Admin,
  timeZone: TimeZone.Local,
};

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
  user: DEV_USER,
};

const engine = new Styletron();
const queryClient = new QueryClient();

export function App() {
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
