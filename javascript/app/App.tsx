import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { CoreApp } from '@uber/michelangelo-core';
import { request } from '@michelangelo/rpc';

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
