import { createContext } from 'react';

import type { ServiceContextType } from './types';

export const ServiceContext = createContext<ServiceContextType>({
  useQuery: () => {
    throw new Error('useQuery must be used within a QueryProvider');
  },
});
