import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { request } from '@michelangelo/rpc';
import { CoreApp } from '@uber/michelangelo-core';

const dependencies = {
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
