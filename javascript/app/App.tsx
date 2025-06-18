import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { request } from '@michelangelo/rpc';
import { CoreApp } from '@uber/michelangelo-core';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { ICONS } from './icons/icons';

const dependencies = {
  theme: {
    icons: ICONS,
  },
  service: {
    request,
  },
};

const engine = new Styletron();

export function App() {
  return (
    <StyletronProvider value={engine}>
      <BrowserRouter>
        <Routes>
          <Route path="/*" element={<CoreApp dependencies={dependencies} />} />
        </Routes>
      </BrowserRouter>
    </StyletronProvider>
  );
}
