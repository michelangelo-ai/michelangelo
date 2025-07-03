import { createContext } from 'react';

import type { CellContextType } from './types';

export const CellContext = createContext<CellContextType | null>(null);
