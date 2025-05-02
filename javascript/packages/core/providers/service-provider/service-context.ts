import { createContext } from 'react';

import type { ServiceContextType } from './types';

export const ServiceContext = createContext<ServiceContextType>({
  request: () => {
    throw new Error('request must be referenced within a ServiceProvider');
  },
});
