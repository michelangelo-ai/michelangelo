import { createContext } from 'react';

import type { QueryContextType } from './types';

export const QueryContext = createContext<QueryContextType>({
  useQuery: () => {
    throw new Error('useQuery must be used within a QueryProvider');
  },
});
