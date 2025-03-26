import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { CoreApp } from '@michelangelo/core';

import './index.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <CoreApp />
  </StrictMode>
);
