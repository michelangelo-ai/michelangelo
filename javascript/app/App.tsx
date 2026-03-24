import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { normalizeConnectError, request } from '@michelangelo/rpc';
import { CoreApp } from '@michelangelo-ai/core';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { ICONS } from './icons/icons';

const dependencies = {
  error: {
    normalizeError: normalizeConnectError,
  },
  theme: {
    icons: ICONS,
  },
  service: {
    request,
  },
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
