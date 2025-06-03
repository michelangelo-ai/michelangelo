import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { request } from '@michelangelo/rpc';
import { CoreApp } from '@uber/michelangelo-core';

import { ICONS } from './icons/icons';

const dependencies = {
  theme: {
    icons: ICONS,
  },
  service: {
    request,
  },
};

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/*" element={<CoreApp dependencies={dependencies} />} />
      </Routes>
    </BrowserRouter>
  );
}
